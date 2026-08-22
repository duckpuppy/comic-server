// Package covers extracts and caches comic cover thumbnails.
package covers

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif" // register GIF decoding
	"image/jpeg"
	_ "image/png" // register PNG decoding
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "golang.org/x/image/webp" // register WebP decoding

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// DefaultThumbnailWidth is how wide (in pixels) cached cover thumbnails are
// resized to - comics-grid cards don't need a full-resolution cover (real
// covers are often 1000px+ wide), and caching the full size wastes disk and
// bandwidth for no visual benefit at thumbnail display size.
const DefaultThumbnailWidth = 300

// jpegQuality is used when re-encoding thumbnails; covers are photographic/
// illustrated content where this is visually lossless at thumbnail size.
const jpegQuality = 85

// Cache stores resized comic cover thumbnails on disk, keyed by book ID and
// invalidated by the source archive file's mtime+size (encoded into the
// cached file's name) - see comic-server-0y6's design notes for why that
// single signal is sufficient: any change to which image is the cover
// requires rewriting the archive file, which always changes its mtime/size.
type Cache struct {
	dir           string
	thumbnailSize int
}

// NewCache creates a Cache rooted at dir, creating it if it doesn't exist.
// thumbnailWidth <= 0 uses DefaultThumbnailWidth.
func NewCache(dir string, thumbnailWidth int) (*Cache, error) {
	if thumbnailWidth <= 0 {
		thumbnailWidth = DefaultThumbnailWidth
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create cover cache dir: %w", err)
	}
	return &Cache{dir: dir, thumbnailSize: thumbnailWidth}, nil
}

// Get returns a resized JPEG thumbnail of bookID's cover, extracted from
// the comic archive at filePath. Serves a cached copy if one exists for the
// archive's current mtime+size; otherwise extracts, resizes, caches, and
// returns the fresh thumbnail. A failure to write the cache doesn't fail
// the call - the freshly extracted thumbnail is still returned.
func (c *Cache) Get(bookID, filePath string) ([]byte, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat comic file: %w", err)
	}

	prefix := sanitizeForFilename(bookID)
	wantPath := filepath.Join(c.dir, cacheFileName(prefix, info.ModTime(), info.Size()))

	if data, err := os.ReadFile(wantPath); err == nil {
		return data, nil
	}

	// Missing or stale (source file changed since it was cached) - drop any
	// old cached entries for this book before rebuilding.
	if stale, err := filepath.Glob(filepath.Join(c.dir, prefix+".*")); err == nil {
		for _, f := range stale {
			os.Remove(f)
		}
	}

	raw, err := comicvine.ExtractCover(filePath)
	if err != nil {
		return nil, err
	}
	thumb, err := resizeToJPEG(raw, c.thumbnailSize)
	if err != nil {
		return nil, err
	}

	writeCacheFile(wantPath, thumb)
	return thumb, nil
}

// Invalidate removes any cached thumbnail for bookID, regardless of its
// recorded mtime/size, forcing the next Get to re-extract. No-op (not an
// error) if nothing was cached.
func (c *Cache) Invalidate(bookID string) error {
	matches, err := filepath.Glob(filepath.Join(c.dir, sanitizeForFilename(bookID)+".*"))
	if err != nil {
		return fmt.Errorf("glob cache entries: %w", err)
	}
	for _, f := range matches {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove cached cover %s: %w", f, err)
		}
	}
	return nil
}

// writeCacheFile best-effort writes data to path via a temp-file-then-rename
// (avoiding a torn read by a concurrent request), ignoring any error - a
// cache write failure shouldn't turn into a failed cover request when the
// thumbnail was already successfully produced.
func writeCacheFile(path string, data []byte) {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path)
}

// cacheFileName builds a filename that encodes the source file's mtime+size,
// so a stale cache entry (source changed) simply doesn't match on lookup.
func cacheFileName(prefix string, modTime time.Time, size int64) string {
	return fmt.Sprintf("%s.%d.%d.jpg", prefix, modTime.UnixNano(), size)
}

// sanitizeForFilename replaces any character that isn't safe to use
// unescaped in a filename on both Linux and Windows with "_". Book IDs are
// GUIDs (often wrapped in braces, e.g. "{...}"), which are otherwise fine on
// Linux but not valid in Windows filenames.
func sanitizeForFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// resizeToJPEG decodes an image (jpg/png/gif/webp) and resizes it down to
// targetWidth (preserving aspect ratio) using nearest-neighbor sampling -
// the same technique already used by internal/comicvine's cover-hash
// resizing, kept intentionally simple since a thumbnail doesn't need
// high-quality interpolation. Images already at or below targetWidth are
// left at their original size (never upscaled), just re-encoded as JPEG so
// the cache format is consistent regardless of the source format.
func resizeToJPEG(data []byte, targetWidth int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode cover image: %w", err)
	}

	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("cover image has invalid dimensions %dx%d", srcW, srcH)
	}

	out := img
	if srcW > targetWidth {
		targetHeight := srcH * targetWidth / srcW
		if targetHeight < 1 {
			targetHeight = 1
		}
		out = resizeNearestNeighbor(img, bounds, srcW, srcH, targetWidth, targetHeight)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}
	return buf.Bytes(), nil
}

func resizeNearestNeighbor(img image.Image, bounds image.Rectangle, srcW, srcH, dstW, dstH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := range dstH {
		srcY := bounds.Min.Y + y*srcH/dstH
		for x := range dstW {
			srcX := bounds.Min.X + x*srcW/dstW
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}
	return dst
}
