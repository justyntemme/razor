//go:build windows

package platform

// This import enables proper thread initialization for Windows callbacks.
// Without CGO, syscall.NewCallback and windows.NewCallback cannot properly
// handle callbacks from Windows threads (like OLE/COM drag-drop).
// See: https://github.com/golang/go/issues/20823

/*
#include <windows.h>
*/
import "C"

// CgoEnabled returns true if CGO is properly linked.
// This function MUST be called from drop_windows.go to force CGO linking.
func CgoEnabled() bool {
	return C.int(1) == 1
}
