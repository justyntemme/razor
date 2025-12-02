//go:build windows

package fs

import (
	"syscall"
	"unsafe"
)

// Drive represents a mounted drive/volume
type Drive struct {
	Name string
	Path string
}

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getLogicalDrives = kernel32.NewProc("GetLogicalDrives")
	getDriveTypeW    = kernel32.NewProc("GetDriveTypeW")
	getVolumeInfoW   = kernel32.NewProc("GetVolumeInformationW")
)

const (
	DRIVE_UNKNOWN     = 0
	DRIVE_NO_ROOT_DIR = 1
	DRIVE_REMOVABLE   = 2
	DRIVE_FIXED       = 3
	DRIVE_REMOTE      = 4
	DRIVE_CDROM       = 5
	DRIVE_RAMDISK     = 6
)

// ListDrivePaths returns drive paths without volume names.
// This is fast and non-blocking - uses GetLogicalDrives API which returns immediately.
// Use this when you only need paths (e.g., for scanning recycle bins).
func ListDrivePaths() []string {
	var paths []string

	// GetLogicalDrives returns immediately without blocking on disconnected drives
	mask, _, _ := getLogicalDrives.Call()
	if mask == 0 {
		return paths
	}

	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		paths = append(paths, string(rune('A'+i))+":\\")
	}

	return paths
}

// ListDrives returns available drives on Windows with display names.
// Note: This may be slow if there are disconnected network drives or empty CD-ROMs,
// as GetVolumeInformationW can block. Call from a goroutine to avoid UI blocking.
func ListDrives() []Drive {
	var drives []Drive

	// Get drive paths first (fast, non-blocking)
	paths := ListDrivePaths()

	for _, path := range paths {
		letter := string(path[0])

		// Get drive type
		pathPtr, _ := syscall.UTF16PtrFromString(path)
		driveType, _, _ := getDriveTypeW.Call(uintptr(unsafe.Pointer(pathPtr)))

		// Skip unknown and no-root drives
		if driveType == DRIVE_UNKNOWN || driveType == DRIVE_NO_ROOT_DIR {
			continue
		}

		// Try to get volume name (this can block on slow/disconnected drives)
		volumeName := make([]uint16, 256)
		ret, _, _ := getVolumeInfoW.Call(
			uintptr(unsafe.Pointer(pathPtr)),
			uintptr(unsafe.Pointer(&volumeName[0])),
			256,
			0, 0, 0, 0, 0,
		)

		name := letter + ":"
		if ret != 0 {
			volName := syscall.UTF16ToString(volumeName)
			if volName != "" {
				name = volName + " (" + letter + ":)"
			}
		}

		// Add drive type indicator
		switch driveType {
		case DRIVE_REMOVABLE:
			if name == letter+":" {
				name = "Removable (" + letter + ":)"
			}
		case DRIVE_CDROM:
			if name == letter+":" {
				name = "CD/DVD (" + letter + ":)"
			}
		case DRIVE_REMOTE:
			if name == letter+":" {
				name = "Network (" + letter + ":)"
			}
		}

		drives = append(drives, Drive{Name: name, Path: path})
	}

	return drives
}
