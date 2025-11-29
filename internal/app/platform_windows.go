//go:build windows

package app

import (
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	shell32       = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW = shell32.NewProc("ShellExecuteW")
)

// platformOpen opens the file using the Windows 'start' command.
func platformOpen(path string) error {
	// 'cmd /c start "" "path"' is the standard way to launch files in Windows
	return exec.Command("cmd", "/c", "start", "", path).Start()
}

// platformOpenWith shows the Windows "Open With" dialog.
// If appPath is specified, opens with that specific application.
// If appPath is empty, shows the system "Open With" dialog.
func platformOpenWith(filePath string, appPath string) error {
	if appPath != "" {
		// Open with specific application
		return exec.Command(appPath, filePath).Start()
	}

	// Try ShellExecute with "openas" verb first
	pathPtr, _ := syscall.UTF16PtrFromString(filePath)
	verbPtr, _ := syscall.UTF16PtrFromString("openas")

	ret, _, _ := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		0,
		1, // SW_SHOWNORMAL
	)

	// ShellExecute returns > 32 on success
	if ret > 32 {
		return nil
	}

	// Fallback: use rundll32 to show "Open With" dialog
	return exec.Command("rundll32", "shell32.dll,OpenAs_RunDLL", filePath).Start()
}

// platformGetApplications returns applications registered to open the given file type.
// On Windows, this returns common applications. Full registry query would be complex.
func platformGetApplications(filePath string) ([]AppInfo, error) {
	var apps []AppInfo

	// Common Windows applications
	commonApps := []struct {
		name string
		exe  string
	}{
		{"Notepad", "notepad.exe"},
		{"WordPad", "write.exe"},
		{"Paint", "mspaint.exe"},
		{"Windows Photo Viewer", "rundll32.exe"},
		{"Visual Studio Code", "code"},
		{"Notepad++", "notepad++"},
	}

	for _, app := range commonApps {
		// Check if the executable exists in PATH
		if _, err := exec.LookPath(app.exe); err == nil {
			apps = append(apps, AppInfo{
				Name: app.name,
				Path: app.exe,
			})
		}
	}

	return apps, nil
}

// AppInfo represents an application that can open files
type AppInfo struct {
	Name string // Display name
	Path string // Executable path or identifier
}
