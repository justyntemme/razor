package ui

import (
	"container/list"
	"image"
	"os"
	"strings"
	"sync"

	"gioui.org/op/paint"
	"golang.org/x/image/draw"

	"github.com/justyntemme/razor/internal/debug"
)

// ThumbnailCache provides an LRU cache for image thumbnails.
// Thumbnails are stored at reduced resolution to minimize memory usage.
type ThumbnailCache struct {
	mu        sync.RWMutex
	cache     map[string]*thumbnailEntry // path -> entry
	lru       *list.List                 // LRU list (front = most recent)
	maxSize   int                        // Maximum number of entries
	maxPixels int                        // Maximum thumbnail dimension (width or height)

	// Pending load requests
	pendingMu sync.Mutex
	pending   map[string]bool // Paths currently being loaded
	loadChan  chan string     // Channel for load requests
	stopChan  chan struct{}   // Channel to stop the loader
}

type thumbnailEntry struct {
	path      string
	thumbnail paint.ImageOp
	size      image.Point  // Original image dimensions
	element   *list.Element
}

// NewThumbnailCache creates a new thumbnail cache.
// maxEntries is the maximum number of thumbnails to cache.
// maxPixels is the maximum dimension (width or height) for thumbnails.
func NewThumbnailCache(maxEntries, maxPixels int) *ThumbnailCache {
	tc := &ThumbnailCache{
		cache:     make(map[string]*thumbnailEntry),
		lru:       list.New(),
		maxSize:   maxEntries,
		maxPixels: maxPixels,
		pending:   make(map[string]bool),
		loadChan:  make(chan string, 100), // Buffer for load requests
		stopChan:  make(chan struct{}),
	}
	// Start background loader
	go tc.backgroundLoader()
	return tc
}

// Get retrieves a thumbnail from the cache.
// Returns the thumbnail, original size, and whether it was found.
func (tc *ThumbnailCache) Get(path string) (paint.ImageOp, image.Point, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	entry, ok := tc.cache[path]
	if !ok {
		return paint.ImageOp{}, image.Point{}, false
	}

	// Move to front of LRU (need write lock)
	tc.mu.RUnlock()
	tc.mu.Lock()
	tc.lru.MoveToFront(entry.element)
	tc.mu.Unlock()
	tc.mu.RLock()

	return entry.thumbnail, entry.size, true
}

// RequestLoad queues a path for background thumbnail loading.
// Does nothing if the path is already cached or being loaded.
func (tc *ThumbnailCache) RequestLoad(path string) {
	// Check if already cached
	tc.mu.RLock()
	_, cached := tc.cache[path]
	tc.mu.RUnlock()
	if cached {
		return
	}

	// Check if already pending
	tc.pendingMu.Lock()
	if tc.pending[path] {
		tc.pendingMu.Unlock()
		return
	}
	tc.pending[path] = true
	tc.pendingMu.Unlock()

	// Queue for loading (non-blocking)
	select {
	case tc.loadChan <- path:
	default:
		// Channel full, drop this request
		tc.pendingMu.Lock()
		delete(tc.pending, path)
		tc.pendingMu.Unlock()
	}
}

// Clear removes all entries from the cache.
func (tc *ThumbnailCache) Clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.cache = make(map[string]*thumbnailEntry)
	tc.lru = list.New()

	// Clear pending
	tc.pendingMu.Lock()
	tc.pending = make(map[string]bool)
	tc.pendingMu.Unlock()

	debug.Log(debug.UI, "ThumbnailCache: cleared")
}

// Stop shuts down the background loader.
func (tc *ThumbnailCache) Stop() {
	close(tc.stopChan)
}

// backgroundLoader processes thumbnail load requests in the background.
func (tc *ThumbnailCache) backgroundLoader() {
	for {
		select {
		case <-tc.stopChan:
			return
		case path := <-tc.loadChan:
			tc.loadThumbnail(path)
		}
	}
}

// loadThumbnail loads and caches a thumbnail for the given path.
func (tc *ThumbnailCache) loadThumbnail(path string) {
	defer func() {
		tc.pendingMu.Lock()
		delete(tc.pending, path)
		tc.pendingMu.Unlock()
	}()

	debug.Log(debug.UI, "ThumbnailCache: loading %s", path)

	// Open and decode the image
	file, err := os.Open(path)
	if err != nil {
		debug.Log(debug.UI, "ThumbnailCache: failed to open %s: %v", path, err)
		return
	}
	defer file.Close()

	var img image.Image
	ext := strings.ToLower(path[strings.LastIndex(path, ".")+1:])

	if ext == "heic" || ext == "heif" {
		if !heicSupported() {
			debug.Log(debug.UI, "ThumbnailCache: HEIC not supported for %s", path)
			return
		}
		img, err = decodeHEIC(file)
	} else {
		img, _, err = image.Decode(file)
	}

	if err != nil {
		debug.Log(debug.UI, "ThumbnailCache: failed to decode %s: %v", path, err)
		return
	}

	originalSize := img.Bounds().Size()

	// Scale down if necessary
	thumbnail := tc.scaleThumbnail(img)

	// Create paint.ImageOp
	imgOp := paint.NewImageOp(thumbnail)

	// Add to cache
	tc.put(path, imgOp, originalSize)

	debug.Log(debug.UI, "ThumbnailCache: cached %s (original %dx%d, thumb %dx%d)",
		path, originalSize.X, originalSize.Y, thumbnail.Bounds().Dx(), thumbnail.Bounds().Dy())
}

// scaleThumbnail scales an image down to fit within maxPixels.
func (tc *ThumbnailCache) scaleThumbnail(src image.Image) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Check if scaling needed
	if width <= tc.maxPixels && height <= tc.maxPixels {
		return src
	}

	// Calculate scale factor
	var scale float64
	if width > height {
		scale = float64(tc.maxPixels) / float64(width)
	} else {
		scale = float64(tc.maxPixels) / float64(height)
	}

	newWidth := int(float64(width) * scale)
	newHeight := int(float64(height) * scale)

	// Create scaled image
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	return dst
}

// put adds a thumbnail to the cache, evicting old entries if necessary.
func (tc *ThumbnailCache) put(path string, thumbnail paint.ImageOp, size image.Point) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Check if already exists
	if entry, ok := tc.cache[path]; ok {
		entry.thumbnail = thumbnail
		entry.size = size
		tc.lru.MoveToFront(entry.element)
		return
	}

	// Evict if at capacity
	for tc.lru.Len() >= tc.maxSize {
		oldest := tc.lru.Back()
		if oldest == nil {
			break
		}
		oldEntry := oldest.Value.(*thumbnailEntry)
		delete(tc.cache, oldEntry.path)
		tc.lru.Remove(oldest)
		debug.Log(debug.UI, "ThumbnailCache: evicted %s", oldEntry.path)
	}

	// Add new entry
	entry := &thumbnailEntry{
		path:      path,
		thumbnail: thumbnail,
		size:      size,
	}
	entry.element = tc.lru.PushFront(entry)
	tc.cache[path] = entry
}

// Size returns the current number of cached thumbnails.
func (tc *ThumbnailCache) Size() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return len(tc.cache)
}
