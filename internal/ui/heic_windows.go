//go:build windows

package ui

import (
	"fmt"
	"image"
	"io"
)

// decodeHEIC is a stub for Windows where HEIC decoding is not supported
func decodeHEIC(r io.Reader) (image.Image, error) {
	return nil, fmt.Errorf("HEIC decoding not supported on Windows")
}

// heicSupported returns whether HEIC decoding is available on this platform
func heicSupported() bool {
	return false
}
