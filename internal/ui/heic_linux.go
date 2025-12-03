//go:build linux

package ui

import (
	"image"
	"io"

	"github.com/jdeng/goheif"
)

// decodeHEIC decodes a HEIC/HEIF image file
func decodeHEIC(r io.Reader) (image.Image, error) {
	return goheif.Decode(r)
}

// heicSupported returns whether HEIC decoding is available on this platform
func heicSupported() bool {
	return true
}
