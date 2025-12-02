//go:build windows

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/justyntemme/razor/internal/fs"
)

// Windows uses the Recycle Bin via shell32.dll SHFileOperationW.
// The Recycle Bin stores files in hidden $Recycle.Bin folders on each drive.
// We use the Shell API to properly interact with it.

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperationW     = shell32.NewProc("SHFileOperationW")
	procSHEmptyRecycleBinW   = shell32.NewProc("SHEmptyRecycleBinW")
	procSHQueryRecycleBinW   = shell32.NewProc("SHQueryRecycleBinW")
)

// SHFILEOPSTRUCTW for SHFileOperationW
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	FO_DELETE = 0x0003
	FOF_ALLOWUNDO = 0x0040
	FOF_NOCONFIRMATION = 0x0010
	FOF_NOERRORUI = 0x0400
	FOF_SILENT = 0x0004

	SHERB_NOCONFIRMATION = 0x00000001
	SHERB_NOPROGRESSUI   = 0x00000002
	SHERB_NOSOUND        = 0x00000004
)

// SHQUERYRBINFO for SHQueryRecycleBinW
type shQueryRBInfo struct {
	cbSize      uint32
	i64Size     int64
	i64NumItems int64
}

func getPath() string {
	// Windows Recycle Bin is virtual - return a marker path
	return "shell:RecycleBinFolder"
}

func isAvailable() bool {
	// Recycle Bin is always available on Windows
	return true
}

func moveToTrash(path string) error {
	// Convert path to absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// SHFileOperationW requires double-null-terminated string
	from, err := syscall.UTF16PtrFromString(absPath + "\x00")
	if err != nil {
		return err
	}

	op := shFileOpStruct{
		wFunc:  FO_DELETE,
		pFrom:  from,
		fFlags: FOF_ALLOWUNDO | FOF_NOCONFIRMATION | FOF_NOERRORUI | FOF_SILENT,
	}

	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 {
		return fmt.Errorf("SHFileOperationW failed with code %d", ret)
	}

	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("operation was aborted")
	}

	return nil
}

func restore(item Item) error {
	// Windows Shell API doesn't provide a direct restore API
	// We would need to use IShellItem2::InvokeVerb("undelete")
	// For now, return an error suggesting manual restore
	return fmt.Errorf("restore from Recycle Bin requires manual action via Windows Explorer")
}

func restoreTo(item Item, destPath string) error {
	// Similar limitation as restore
	return fmt.Errorf("restore from Recycle Bin requires manual action via Windows Explorer")
}

func list() ([]Item, error) {
	// Listing Recycle Bin contents requires IShellFolder enumeration
	// This is complex COM code. For now, we'll scan the $Recycle.Bin folders
	// on accessible drives.

	var items []Item

	// Use fs.ListDrivePaths for fast, non-blocking drive enumeration
	drives := fs.ListDrivePaths()

	for _, drive := range drives {
		recyclePath := filepath.Join(drive, "$Recycle.Bin")
		driveItems, err := scanRecycleBin(recyclePath)
		if err != nil {
			continue // Skip drives we can't access
		}
		items = append(items, driveItems...)
	}

	return items, nil
}

func scanRecycleBin(recyclePath string) ([]Item, error) {
	var items []Item

	// $Recycle.Bin contains SID-named folders for each user
	sidFolders, err := os.ReadDir(recyclePath)
	if err != nil {
		return nil, err
	}

	for _, sidFolder := range sidFolders {
		if !sidFolder.IsDir() || !strings.HasPrefix(sidFolder.Name(), "S-") {
			continue
		}

		sidPath := filepath.Join(recyclePath, sidFolder.Name())
		entries, err := os.ReadDir(sidPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			// Skip $I files (metadata) - we only want $R files (actual data)
			if strings.HasPrefix(entry.Name(), "$I") {
				continue
			}

			if !strings.HasPrefix(entry.Name(), "$R") {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			fullPath := filepath.Join(sidPath, entry.Name())

			// Try to get original name from $I file
			originalName := entry.Name()
			iFileName := "$I" + strings.TrimPrefix(entry.Name(), "$R")
			iFilePath := filepath.Join(sidPath, iFileName)
			if origName, origPath, delTime := parseRecycleBinInfo(iFilePath); origName != "" {
				originalName = origName
				items = append(items, Item{
					Name:         originalName,
					OriginalPath: origPath,
					TrashPath:    fullPath,
					DeletedAt:    delTime,
					Size:         info.Size(),
					IsDir:        entry.IsDir(),
				})
			} else {
				items = append(items, Item{
					Name:      originalName,
					TrashPath: fullPath,
					DeletedAt: info.ModTime(),
					Size:      info.Size(),
					IsDir:     entry.IsDir(),
				})
			}
		}
	}

	return items, nil
}

func parseRecycleBinInfo(iFilePath string) (name string, originalPath string, deletedAt time.Time) {
	data, err := os.ReadFile(iFilePath)
	if err != nil || len(data) < 28 {
		return "", "", time.Time{}
	}

	// $I file format (Windows Vista+):
	// Bytes 0-7: Header (version)
	// Bytes 8-15: Original file size
	// Bytes 16-23: Deletion timestamp (FILETIME)
	// Bytes 24-27: Length of original path (in characters)
	// Bytes 28+: Original path (UTF-16LE)

	// Read deletion timestamp (FILETIME - 100-nanosecond intervals since Jan 1, 1601)
	ft := uint64(data[16]) | uint64(data[17])<<8 | uint64(data[18])<<16 | uint64(data[19])<<24 |
		uint64(data[20])<<32 | uint64(data[21])<<40 | uint64(data[22])<<48 | uint64(data[23])<<56

	// Convert FILETIME to Go time
	// FILETIME epoch: January 1, 1601
	// Unix epoch: January 1, 1970
	// Difference: 116444736000000000 (100-nanosecond intervals)
	const filetimeEpochDiff = 116444736000000000
	if ft > filetimeEpochDiff {
		unixNano := (int64(ft) - filetimeEpochDiff) * 100
		deletedAt = time.Unix(0, unixNano)
	}

	// Read path length
	if len(data) < 28 {
		return "", "", deletedAt
	}
	pathLen := uint32(data[24]) | uint32(data[25])<<8 | uint32(data[26])<<16 | uint32(data[27])<<24

	// Read original path (UTF-16LE)
	pathBytes := data[28:]
	if uint32(len(pathBytes)) < pathLen*2 {
		return "", "", deletedAt
	}

	// Convert UTF-16LE to string
	utf16Chars := make([]uint16, pathLen)
	for i := uint32(0); i < pathLen && int(i*2+1) < len(pathBytes); i++ {
		utf16Chars[i] = uint16(pathBytes[i*2]) | uint16(pathBytes[i*2+1])<<8
	}

	originalPath = syscall.UTF16ToString(utf16Chars)
	name = filepath.Base(originalPath)

	return name, originalPath, deletedAt
}

func empty() error {
	// SHEmptyRecycleBinW empties the Recycle Bin
	ret, _, _ := procSHEmptyRecycleBinW.Call(
		0, // hwnd
		0, // pszRootPath (NULL = all drives)
		SHERB_NOCONFIRMATION|SHERB_NOPROGRESSUI|SHERB_NOSOUND,
	)

	if ret != 0 {
		// S_OK is 0, E_UNEXPECTED can happen if already empty
		// Just ignore errors for empty operation
	}

	return nil
}

func deleteItem(item Item) error {
	// Permanently delete a specific item from Recycle Bin
	return os.RemoveAll(item.TrashPath)
}

func displayName() string {
	return "Recycle Bin"
}
