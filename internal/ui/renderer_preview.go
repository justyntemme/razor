package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/op/paint"

	"github.com/justyntemme/razor/internal/debug"
)

// File preview loading and state management

// ShowPreview loads and displays the preview pane for the given file
func (r *Renderer) ShowPreview(path string) error {
	debug.Log(debug.UI, "ShowPreview called for: %s", path)

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	debug.Log(debug.UI, "ShowPreview: ext=%s, textExts=%v, imageExts=%v", ext, r.previewExtensions, r.previewImageExts)

	// Check if it's a text file
	isText := false
	for _, e := range r.previewExtensions {
		if strings.ToLower(e) == ext {
			isText = true
			break
		}
	}

	// Check if it's an image file
	isImage := false
	for _, e := range r.previewImageExts {
		if strings.ToLower(e) == ext {
			isImage = true
			break
		}
	}

	debug.Log(debug.UI, "ShowPreview: isText=%v, isImage=%v", isText, isImage)

	if !isText && !isImage {
		debug.Log(debug.UI, "ShowPreview: hiding preview (not text or image)")
		r.HidePreview()
		return nil
	}

	// Check file size
	info, err := os.Stat(path)
	if err != nil {
		r.previewError = fmt.Sprintf("Cannot access file: %v", err)
		r.previewVisible = true
		r.previewPath = path
		r.previewContent = ""
		r.previewIsImage = false
		return err
	}
	if info.IsDir() {
		r.HidePreview()
		return nil
	}
	// For text files, check file size limit (images are scaled so no limit needed)
	if !isImage && r.previewMaxSize > 0 && info.Size() > r.previewMaxSize {
		r.previewError = fmt.Sprintf("File too large (%s)", formatSize(info.Size()))
		r.previewVisible = true
		r.previewPath = path
		r.previewContent = ""
		r.previewIsImage = false
		return nil
	}

	r.previewPath = path
	r.previewError = ""

	if isImage {
		// Load image (will be scaled to fit preview pane)
		return r.loadImagePreview(path)
	}

	// Load text content
	return r.loadTextPreview(path, ext)
}

// loadImagePreview loads an image file for preview
func (r *Renderer) loadImagePreview(path string) error {
	debug.Log(debug.UI, "loadImagePreview: loading %s", path)

	file, err := os.Open(path)
	if err != nil {
		debug.Log(debug.UI, "loadImagePreview: cannot open file: %v", err)
		r.previewError = fmt.Sprintf("Cannot open file: %v", err)
		r.previewVisible = true
		r.previewIsImage = false
		return err
	}
	defer file.Close()

	var img image.Image

	// Check if it's a HEIC/HEIF file
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".heic" || ext == ".heif" {
		if !heicSupported() {
			debug.Log(debug.UI, "loadImagePreview: HEIC not supported on this platform")
			r.previewError = "HEIC preview not supported on this platform"
			r.previewVisible = true
			r.previewIsImage = false
			return fmt.Errorf("HEIC not supported")
		}
		debug.Log(debug.UI, "loadImagePreview: decoding HEIC/HEIF")
		img, err = decodeHEIC(file)
		if err != nil {
			debug.Log(debug.UI, "loadImagePreview: HEIC decode error: %v", err)
			r.previewError = fmt.Sprintf("Cannot decode HEIC: %v", err)
			r.previewVisible = true
			r.previewIsImage = false
			return err
		}
	} else {
		debug.Log(debug.UI, "loadImagePreview: decoding standard image format")
		img, _, err = image.Decode(file)
		if err != nil {
			debug.Log(debug.UI, "loadImagePreview: decode error: %v", err)
			r.previewError = fmt.Sprintf("Cannot decode image: %v", err)
			r.previewVisible = true
			r.previewIsImage = false
			return err
		}
	}

	debug.Log(debug.UI, "loadImagePreview: decoded successfully, size=%v", img.Bounds().Size())
	r.previewImage = paint.NewImageOp(img)
	r.previewImageSize = img.Bounds().Size()
	r.previewIsImage = true
	r.previewIsJSON = false
	r.previewContent = ""
	r.previewVisible = true
	debug.Log(debug.UI, "loadImagePreview: previewVisible=%v, previewIsImage=%v, previewImageSize=%v",
		r.previewVisible, r.previewIsImage, r.previewImageSize)
	return nil
}

// loadTextPreview loads a text file for preview
func (r *Renderer) loadTextPreview(path, ext string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		r.previewError = fmt.Sprintf("Cannot read file: %v", err)
		r.previewVisible = true
		r.previewIsImage = false
		r.previewContent = ""
		return err
	}

	r.previewIsImage = false
	r.previewIsJSON = ext == ".json"
	r.previewIsMarkdown = ext == ".md" || ext == ".markdown"
	r.previewIsOrgmode = ext == ".org"

	// Format JSON with indentation
	if r.previewIsJSON {
		var jsonData interface{}
		if err := json.Unmarshal(data, &jsonData); err == nil {
			if formatted, err := json.MarshalIndent(jsonData, "", "  "); err == nil {
				r.previewContent = string(formatted)
			} else {
				r.previewContent = string(data)
			}
		} else {
			r.previewContent = string(data)
			r.previewError = "Invalid JSON: " + err.Error()
		}
	} else {
		r.previewContent = string(data)
	}

	// Parse markdown if this is a markdown file
	if r.previewIsMarkdown {
		r.previewMarkdownBlocks = ParseMarkdown(string(data))
		// previewMarkdownRender is already set from config via SetPreviewConfig
	} else if r.previewIsOrgmode {
		r.previewOrgmodeBlocks = ParseOrgMode(string(data))
		// previewOrgmodeRender is already set from config via SetPreviewConfig
	} else {
		r.previewMarkdownBlocks = nil
		r.previewOrgmodeBlocks = nil
	}

	r.previewVisible = true
	return nil
}

// HidePreview hides the preview pane
func (r *Renderer) HidePreview() {
	r.previewVisible = false
	r.previewPath = ""
	r.previewContent = ""
	r.previewError = ""
	r.previewIsImage = false
	r.previewImage = paint.ImageOp{}
	r.previewImageSize = image.Point{}
	r.previewIsMarkdown = false
	r.previewMarkdownBlocks = nil
	r.previewIsOrgmode = false
	r.previewOrgmodeBlocks = nil
}

// IsPreviewVisible returns whether the preview pane is currently shown
func (r *Renderer) IsPreviewVisible() bool {
	return r.previewVisible
}

// SetRecentView sets whether we're viewing recent files
func (r *Renderer) SetRecentView(isRecent bool) {
	r.isRecentView = isRecent
	if isRecent {
		r.isTrashView = false // Can't be in both views
	}
}

// IsRecentView returns whether we're viewing recent files
func (r *Renderer) IsRecentView() bool {
	return r.isRecentView
}
