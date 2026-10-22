package cbzconvert

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // register GIF decoding
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP decoding

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// Page output formats and resize modes (tracker #7, step 2).
const (
	PageFormatKeep = ""
	PageFormatJPEG = "jpeg"
	PageFormatPNG  = "png"

	PageResizeNone   = ""
	PageResizeWidth  = "width"  // scale to Width, keep the shape
	PageResizeHeight = "height" // scale to Height, keep the shape
	PageResizeFit    = "fit"    // fit inside Width x Height, keep the shape

	DefaultPageQuality = 85
	MaxPageDimension   = 20000
	// maxDecodePixels refuses a page that would need a huge buffer to decode.
	maxDecodePixels = 100_000_000
)

// PageOptions say how an export copy's pages are re-encoded or resized. The
// zero value changes nothing: pages are copied byte for byte.
type PageOptions struct {
	Format      string // PageFormat*
	Quality     int    // JPEG quality 1-100; 0 = DefaultPageQuality
	Resize      string // PageResize*
	Width       int
	Height      int
	DontEnlarge bool // never scale a page up
}

// Active reports whether any page would be touched.
func (o PageOptions) Active() bool { return o.Format != PageFormatKeep || o.Resize != PageResizeNone }

// Validate checks the options a request carries.
func (o PageOptions) Validate() error {
	switch o.Format {
	case PageFormatKeep, PageFormatJPEG, PageFormatPNG:
	default:
		return fmt.Errorf("format must be empty, %q or %q", PageFormatJPEG, PageFormatPNG)
	}
	if o.Quality < 0 || o.Quality > 100 {
		return fmt.Errorf("quality must be 1-100")
	}
	dim := func(name string, v int) error {
		if v < 1 || v > MaxPageDimension {
			return fmt.Errorf("%s must be 1-%d", name, MaxPageDimension)
		}
		return nil
	}
	switch o.Resize {
	case PageResizeNone:
	case PageResizeWidth:
		return dim("width", o.Width)
	case PageResizeHeight:
		return dim("height", o.Height)
	case PageResizeFit:
		if err := dim("width", o.Width); err != nil {
			return err
		}
		return dim("height", o.Height)
	default:
		return fmt.Errorf("resize must be empty, %q, %q or %q", PageResizeWidth, PageResizeHeight, PageResizeFit)
	}
	return nil
}

// PageStat is the size of one page after TransformPages touched it.
type PageStat struct {
	Changed       bool
	Size          int64
	Width, Height int
}

// TransformPages applies opts to every page. Pages the options do not change
// keep their bytes. When any option is active the pages are renamed to a
// zero-padded sequence (000.jpg ...): a format change would otherwise make
// names clash, and readers order by name. The returned stats line up with pages.
func TransformPages(pages []comicvine.Page, opts PageOptions) ([]comicvine.Page, []PageStat, error) {
	if !opts.Active() {
		return pages, make([]PageStat, len(pages)), nil
	}
	if err := opts.Validate(); err != nil {
		return nil, nil, err
	}
	out := make([]comicvine.Page, len(pages))
	stats := make([]PageStat, len(pages))
	width := len(fmt.Sprint(len(pages)))
	if width < 3 {
		width = 3
	}
	for i, p := range pages {
		data, ext, st, err := transformPage(p, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("page %d (%s): %w", i+1, filepath.Base(p.Name), err)
		}
		if ext == "" {
			ext = strings.ToLower(filepath.Ext(p.Name))
		}
		out[i] = comicvine.Page{Name: fmt.Sprintf("%0*d%s", width, i, ext), Data: data}
		stats[i] = st
	}
	return out, stats, nil
}

// transformPage returns the new bytes and extension for one page. ext is empty
// when the bytes are the original ones.
func transformPage(p comicvine.Page, opts PageOptions) ([]byte, string, PageStat, error) {
	cfg, srcFormat, err := image.DecodeConfig(bytes.NewReader(p.Data))
	if err != nil {
		return nil, "", PageStat{}, fmt.Errorf("not a readable image: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > maxDecodePixels {
		return nil, "", PageStat{}, fmt.Errorf("image size %dx%d is not supported", cfg.Width, cfg.Height)
	}

	dstW, dstH := targetSize(cfg.Width, cfg.Height, opts)
	resize := dstW != cfg.Width || dstH != cfg.Height
	outFormat := opts.Format
	if outFormat == PageFormatKeep {
		switch srcFormat {
		case "jpeg":
			outFormat = PageFormatJPEG
		case "png":
			outFormat = PageFormatPNG
		case "gif":
			outFormat = PageFormatPNG
		default: // webp
			outFormat = PageFormatJPEG
		}
	}
	sameFormat := outFormat == srcFormat
	if !resize && sameFormat {
		return p.Data, "", PageStat{}, nil // nothing to change: keep the exact bytes
	}

	img, _, err := image.Decode(bytes.NewReader(p.Data))
	if err != nil {
		return nil, "", PageStat{}, fmt.Errorf("decode: %w", err)
	}
	if resize {
		dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		img = dst
	}

	var buf bytes.Buffer
	ext := ".png"
	if outFormat == PageFormatJPEG {
		ext = ".jpg"
		q := opts.Quality
		if q == 0 {
			q = DefaultPageQuality
		}
		if err := jpeg.Encode(&buf, flattenOnWhite(img), &jpeg.Options{Quality: q}); err != nil {
			return nil, "", PageStat{}, fmt.Errorf("encode JPEG: %w", err)
		}
	} else if err := png.Encode(&buf, img); err != nil {
		return nil, "", PageStat{}, fmt.Errorf("encode PNG: %w", err)
	}
	b := img.Bounds()
	return buf.Bytes(), ext, PageStat{Changed: true, Size: int64(buf.Len()), Width: b.Dx(), Height: b.Dy()}, nil
}

// targetSize works out the size a page is scaled to; the source size when the
// options leave it alone.
func targetSize(w, h int, o PageOptions) (int, int) {
	var scale float64
	switch o.Resize {
	case PageResizeWidth:
		scale = float64(o.Width) / float64(w)
	case PageResizeHeight:
		scale = float64(o.Height) / float64(h)
	case PageResizeFit:
		sw, sh := float64(o.Width)/float64(w), float64(o.Height)/float64(h)
		scale = sw
		if sh < sw {
			scale = sh
		}
	default:
		return w, h
	}
	if scale == 1 || (scale > 1 && o.DontEnlarge) {
		return w, h
	}
	nw, nh := int(float64(w)*scale+0.5), int(float64(h)*scale+0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	return nw, nh
}

// flattenOnWhite puts a transparent image on white, since JPEG has no alpha.
func flattenOnWhite(img image.Image) image.Image {
	if op, ok := img.(interface{ Opaque() bool }); ok && op.Opaque() {
		return img
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	xdraw.Draw(dst, b, image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.Draw(dst, b, img, b.Min, xdraw.Over)
	return dst
}
