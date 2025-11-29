//go:build linux

package fs

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Drive represents a mounted drive/volume
type Drive struct {
	Name string
	Path string
}

// ListDrives returns mounted drives on Linux
func ListDrives() []Drive {
	var drives []Drive

	// Always include root
	drives = append(drives, Drive{Name: "/ (Root)", Path: "/"})

	// Parse /proc/mounts for mounted filesystems
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return drives
	}
	defer file.Close()

	seen := make(map[string]bool)
	seen["/"] = true

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		mountPoint := fields[1]
		fsType := ""
		if len(fields) >= 3 {
			fsType = fields[2]
		}

		// Skip virtual filesystems
		if strings.HasPrefix(mountPoint, "/sys") ||
			strings.HasPrefix(mountPoint, "/proc") ||
			strings.HasPrefix(mountPoint, "/dev") ||
			strings.HasPrefix(mountPoint, "/run") ||
			strings.HasPrefix(mountPoint, "/snap") ||
			fsType == "tmpfs" ||
			fsType == "devtmpfs" ||
			fsType == "cgroup" ||
			fsType == "cgroup2" {
			continue
		}

		// Include /home, /media/*, /mnt/* and other real mounts
		if seen[mountPoint] {
			continue
		}

		// Determine display name
		name := mountPoint
		if strings.HasPrefix(mountPoint, "/media/") || strings.HasPrefix(mountPoint, "/mnt/") {
			name = filepath.Base(mountPoint)
		} else if mountPoint == "/home" {
			name = "Home"
		}

		seen[mountPoint] = true
		drives = append(drives, Drive{Name: name, Path: mountPoint})
	}

	return drives
}
