package comicvine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/bits"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bodgit/sevenzip"
	rardecode "github.com/nwaples/rardecode/v2"
	_ "golang.org/x/image/webp"
)

// CoverHash is a 64-bit difference hash (dHash) of a cover image, used for
// fast, approximate cover-art comparison during ComicVine matching.
type CoverHash uint64

// Similarity returns how alike two cover hashes are, in [0.0, 1.0].
// 1.0 means identical (or indistinguishable at this hash resolution);
// 0.0 means maximally different.
func (h CoverHash) Similarity(other CoverHash) float64 {
	dist := bits.OnesCount64(uint64(h ^ other))
	return 1.0 - float64(dist)/64.0
}

// Cover-match thresholds, ported from comic-vine-scraper's automatcher.py.
const (
	// CoverVerifyThreshold is the similarity above which a candidate's cover
	// is considered a confirmed match against the local comic's cover.
	CoverVerifyThreshold = 0.87

	// AmbiguousCoverThreshold flags two top candidates as indistinguishable
	// by cover art (e.g. a single issue vs. its TPB collection) when their
	// own covers are at least this similar to each other.
	AmbiguousCoverThreshold = 0.77
)

// ComputeDHash computes a difference hash from raw image bytes (jpg/png/gif).
func ComputeDHash(data []byte) (CoverHash, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("decode image: %w", err)
	}
	return dHashImage(img), nil
}

// dHashImage computes a difference hash: resize to 9x8 grayscale, then set
// each bit based on whether a pixel is brighter than its right neighbor.
func dHashImage(img image.Image) CoverHash {
	const w, h = 9, 8
	gray := resizeGrayscale(img, w, h)

	var hash uint64
	for y := range h {
		for x := range w - 1 {
			hash <<= 1
			if gray[y][x] < gray[y][x+1] {
				hash |= 1
			}
		}
	}
	return CoverHash(hash)
}

// resizeGrayscale downsamples img to w x h using nearest-neighbor sampling
// and converts each sample to 8-bit luminance.
func resizeGrayscale(img image.Image, w, h int) [][]uint8 {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW == 0 {
		srcW = 1
	}
	if srcH == 0 {
		srcH = 1
	}

	out := make([][]uint8, h)
	for y := range h {
		out[y] = make([]uint8, w)
		srcY := bounds.Min.Y + y*srcH/h
		for x := range w {
			srcX := bounds.Min.X + x*srcW/w
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			lum := (299*r + 587*g + 114*b) / 1000
			out[y][x] = uint8(lum >> 8)
		}
	}
	return out
}

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

// ExtractCover returns the raw bytes of a comic archive's cover image: the
// page marked Type="FrontCover" in ComicInfo.xml if present, otherwise the
// first image file in alphabetical order. Supports CBZ (zip), CBR (RAR), and
// CB7 (7-Zip) archives, dispatched by file extension.
func ExtractCover(path string) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cbz", ".zip":
		return ExtractCoverFromCBZ(path)
	case ".cbr", ".rar":
		return extractCoverFromCBR(path)
	case ".cb7", ".7z":
		return extractCoverFromCB7(path)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", path)
	}
}

// Page is one image file read from a comic archive, in reading order.
type Page struct {
	Name string // original entry name/path inside the archive
	Data []byte
}

// ReadAllPages returns every image file in a comic archive, sorted the same
// way ExtractCover picks its single cover (alphabetical by entry name).
// Supports CBZ (zip), CBR (RAR), and CB7 (7-Zip), dispatched by file
// extension - shares the per-format listing/reading helpers ExtractCover
// already uses. Non-image entries (ComicInfo.xml, thumbs.db, etc.) are
// skipped. Used by internal/cbzconvert (comic-server-43b) to repack an
// archive's pages into a new CBZ.
func ReadAllPages(path string) ([]Page, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cbz", ".zip":
		return readAllPagesFromCBZ(path)
	case ".cbr", ".rar":
		return readAllPagesFromCBR(path)
	case ".cb7", ".7z":
		return readAllPagesFromCB7(path)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", path)
	}
}

func readAllPagesFromCBZ(path string) ([]Page, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	defer zr.Close()

	images := sortedImageFiles(zr.File)
	pages := make([]Page, 0, len(images))
	for _, f := range images {
		data, err := readZipFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f.Name, err)
		}
		pages = append(pages, Page{Name: f.Name, Data: data})
	}
	return pages, nil
}

func readAllPagesFromCBR(path string) ([]Page, error) {
	files, err := rardecode.List(path)
	if err != nil {
		return nil, fmt.Errorf("open cbr: %w", err)
	}

	images := sortedRARFiles(files)
	pages := make([]Page, 0, len(images))
	for _, f := range images {
		data, err := readRARFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f.Name, err)
		}
		pages = append(pages, Page{Name: f.Name, Data: data})
	}
	return pages, nil
}

func readAllPagesFromCB7(path string) ([]Page, error) {
	rc, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open cb7: %w", err)
	}
	defer rc.Close()

	images := sortedSevenZipFiles(rc.File)
	pages := make([]Page, 0, len(images))
	for _, f := range images {
		data, err := readSevenZipFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f.Name, err)
		}
		pages = append(pages, Page{Name: f.Name, Data: data})
	}
	return pages, nil
}

// chooseCoverIndex picks which of numImages sorted image entries is the
// cover: the ComicInfo.xml FrontCover index if it's valid, else the first
// image.
func chooseCoverIndex(numImages, fcIdx int, fcOK bool) int {
	if fcOK && fcIdx >= 0 && fcIdx < numImages {
		return fcIdx
	}
	return 0
}

type comicInfoXML struct {
	XMLName xml.Name `xml:"ComicInfo"`
	Pages   struct {
		Page []struct {
			Image int    `xml:"Image,attr"`
			Type  string `xml:"Type,attr"`
		} `xml:"Page"`
	} `xml:"Pages"`
}

// frontCoverIndexFromXML parses ComicInfo.xml bytes and returns the page
// index of the entry marked Type="FrontCover", if any.
func frontCoverIndexFromXML(data []byte) (int, bool) {
	var ci comicInfoXML
	if err := xml.Unmarshal(data, &ci); err != nil {
		return 0, false
	}
	for _, p := range ci.Pages.Page {
		if strings.EqualFold(p.Type, "FrontCover") {
			return p.Image, true
		}
	}
	return 0, false
}

// ExtractCoverFromCBZ returns the raw bytes of a CBZ (zip) comic's cover image.
func ExtractCoverFromCBZ(path string) ([]byte, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	defer zr.Close()

	images := sortedImageFiles(zr.File)
	if len(images) == 0 {
		return nil, fmt.Errorf("no image files found in %s", path)
	}

	idx := 0
	if fcIdx, ok := frontCoverIndex(zr.File); ok {
		idx = chooseCoverIndex(len(images), fcIdx, ok)
	}
	return readZipFile(images[idx])
}

func sortedImageFiles(files []*zip.File) []*zip.File {
	var out []*zip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		if imageExtensions[strings.ToLower(filepath.Ext(f.Name))] {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// frontCoverIndex looks for ComicInfo.xml in the archive and returns the
// page index of the entry marked Type="FrontCover", if any.
func frontCoverIndex(files []*zip.File) (int, bool) {
	for _, f := range files {
		if !strings.EqualFold(filepath.Base(f.Name), "ComicInfo.xml") {
			continue
		}
		data, err := readZipFile(f)
		if err != nil {
			return 0, false
		}
		return frontCoverIndexFromXML(data)
	}
	return 0, false
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// extractCoverFromCBR returns the raw bytes of a CBR (RAR) comic's cover
// image. Solid RAR archives aren't supported for random access by the
// underlying decoder; comics are essentially never packed solid since it
// breaks page-random-access for readers too, so this only returns an error
// in that rare case rather than falling back to a full sequential decode.
func extractCoverFromCBR(path string) ([]byte, error) {
	files, err := rardecode.List(path)
	if err != nil {
		return nil, fmt.Errorf("open cbr: %w", err)
	}

	images := sortedRARFiles(files)
	if len(images) == 0 {
		return nil, fmt.Errorf("no image files found in %s", path)
	}

	idx := 0
	if f := findRARFile(files, "ComicInfo.xml"); f != nil {
		if data, err := readRARFile(f); err == nil {
			if fcIdx, ok := frontCoverIndexFromXML(data); ok {
				idx = chooseCoverIndex(len(images), fcIdx, ok)
			}
		}
	}

	return readRARFile(images[idx])
}

func sortedRARFiles(files []*rardecode.File) []*rardecode.File {
	var out []*rardecode.File
	for _, f := range files {
		if f.IsDir {
			continue
		}
		if imageExtensions[strings.ToLower(filepath.Ext(f.Name))] {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func findRARFile(files []*rardecode.File, name string) *rardecode.File {
	for _, f := range files {
		if strings.EqualFold(filepath.Base(f.Name), name) {
			return f
		}
	}
	return nil
}

func readRARFile(f *rardecode.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", f.Name, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// extractCoverFromCB7 returns the raw bytes of a CB7 (7-Zip) comic's cover image.
func extractCoverFromCB7(path string) ([]byte, error) {
	rc, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open cb7: %w", err)
	}
	defer rc.Close()

	images := sortedSevenZipFiles(rc.File)
	if len(images) == 0 {
		return nil, fmt.Errorf("no image files found in %s", path)
	}

	idx := 0
	if f := findSevenZipFile(rc.File, "ComicInfo.xml"); f != nil {
		if data, err := readSevenZipFile(f); err == nil {
			if fcIdx, ok := frontCoverIndexFromXML(data); ok {
				idx = chooseCoverIndex(len(images), fcIdx, ok)
			}
		}
	}

	return readSevenZipFile(images[idx])
}

func sortedSevenZipFiles(files []*sevenzip.File) []*sevenzip.File {
	var out []*sevenzip.File
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		if imageExtensions[strings.ToLower(filepath.Ext(f.Name))] {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func findSevenZipFile(files []*sevenzip.File, name string) *sevenzip.File {
	for _, f := range files {
		if strings.EqualFold(filepath.Base(f.Name), name) {
			return f
		}
	}
	return nil
}

func readSevenZipFile(f *sevenzip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", f.Name, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// DownloadCoverHash fetches an image from url and computes its cover hash.
// Cover art is served from ComicVine's CDN, not the rate-limited API, so
// this bypasses the client's pacer/circuit breaker.
func (c *Client) DownloadCoverHash(ctx context.Context, url string) (CoverHash, error) {
	if url == "" {
		return 0, fmt.Errorf("empty image url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download cover: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read cover: %w", err)
	}
	return ComputeDHash(data)
}
