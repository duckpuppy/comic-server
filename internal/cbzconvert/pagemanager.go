package cbzconvert

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders for DecodeConfig on uploaded pages
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

// Page manager (comic-server-j1cv): reorder, delete and add pages inside a
// CBZ in one atomic write.
//
// Design record:
//   - A plan lists the archive's pages in their NEW order. A page left out is
//     deleted from the file (not just marked Deleted), the original archive
//     going to the trash via trash.Replace.
//   - Kept pages are raw-copied (no recompress) under new zero-padded names
//     (000.jpg, 001.jpg...). Readers order pages by entry name, so renaming is
//     what makes a new order stick.
//   - Page-indexed library data (Pages, CurrentPage, LastPageRead,
//     PreferredFrontCover) is remapped, not reset. A pointer at a deleted page
//     moves to the nearest kept page before it.
//   - ComicInfo.xml (and ComicBook.xml with embed on) is rewritten in the same
//     write, so the file's Pages block never disagrees with the new order.

// PageRef names one page of the new archive: an existing page by its current
// index, or an uploaded image by its position in PagePlan.Uploads.
type PageRef struct {
	Orig   int // current page index; -1 when this is an upload
	Upload int // index into PagePlan.Uploads when Orig < 0
}

// UploadedPage is an image to add.
type UploadedPage struct {
	Ext  string // with the dot, e.g. ".jpg"
	Data []byte
}

// PagePlan is the desired final page list.
type PagePlan struct {
	Order   []PageRef
	Uploads []UploadedPage
}

// PagesResult reports a successful ManagePages. The caller applies it to the
// library record (Pages, PageCount, the three page pointers) and persists it,
// the same division of responsibility as Convert.
type PagesResult struct {
	PageCount           int
	Pages               []library.ComicPageInfo
	CurrentPage         int
	LastPageRead        int
	PreferredFrontCover int
	Deleted             int
	Added               int
}

// ErrNoPageChange means the plan equals the archive's current page list.
var ErrNoPageChange = errors.New("cbzconvert: page plan changes nothing")

// PlanError marks a plan that cannot be applied (bad index, empty list...).
type PlanError struct{ Msg string }

func (e *PlanError) Error() string { return "cbzconvert: invalid page plan: " + e.Msg }

// pageRemap maps the old page numbering onto the new one.
type pageRemap struct {
	newIndex map[int]int // kept original index -> new index
	uploads  []int       // upload k -> new index (-1: unused)
	count    int
}

func buildRemap(plan PagePlan, oldCount int) (pageRemap, error) {
	if len(plan.Order) == 0 {
		return pageRemap{}, &PlanError{"the archive would have no pages"}
	}
	m := pageRemap{newIndex: map[int]int{}, uploads: make([]int, len(plan.Uploads)), count: len(plan.Order)}
	for i := range m.uploads {
		m.uploads[i] = -1
	}
	for pos, ref := range plan.Order {
		if ref.Orig >= 0 {
			if ref.Orig >= oldCount {
				return pageRemap{}, &PlanError{fmt.Sprintf("page %d does not exist (archive has %d)", ref.Orig, oldCount)}
			}
			if _, dup := m.newIndex[ref.Orig]; dup {
				return pageRemap{}, &PlanError{fmt.Sprintf("page %d is listed twice", ref.Orig)}
			}
			m.newIndex[ref.Orig] = pos
			continue
		}
		if ref.Upload < 0 || ref.Upload >= len(plan.Uploads) {
			return pageRemap{}, &PlanError{fmt.Sprintf("upload %d does not exist", ref.Upload)}
		}
		if m.uploads[ref.Upload] >= 0 {
			return pageRemap{}, &PlanError{fmt.Sprintf("upload %d is listed twice", ref.Upload)}
		}
		m.uploads[ref.Upload] = pos
	}
	return m, nil
}

// pointer remaps a page pointer (current page, last page read, preferred
// cover). A deleted page falls back to the nearest kept page before it.
func (m pageRemap) pointer(old int) int {
	for i := old; i >= 0; i-- {
		if n, ok := m.newIndex[i]; ok {
			return n
		}
	}
	return 0
}

func (m pageRemap) unchanged(oldCount int) bool {
	if m.count != oldCount {
		return false
	}
	for old, n := range m.newIndex {
		if old != n {
			return false
		}
	}
	return len(m.newIndex) == m.count
}

// remapPages carries the library's page records onto the new numbering and
// adds a record for every upload. Records of deleted pages are dropped.
func (m pageRemap) remapPages(old []library.ComicPageInfo, uploads []UploadedPage) []library.ComicPageInfo {
	var out []library.ComicPageInfo
	for _, p := range old {
		n, ok := m.newIndex[p.Image]
		if !ok {
			continue
		}
		p.Image = n
		out = append(out, p)
	}
	for k, n := range m.uploads {
		if n < 0 {
			continue
		}
		info := library.ComicPageInfo{Image: n, ImageSize: int64(len(uploads[k].Data))}
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(uploads[k].Data)); err == nil {
			info.ImageWidth, info.ImageHeight = cfg.Width, cfg.Height
		}
		out = append(out, info)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Image < out[j].Image })
	return out
}

// remapComicInfo re-indexes the Pages block of an existing ComicInfo.xml so
// attributes the library does not store (DoublePage...) follow their page.
// Entries of deleted pages are dropped. Unparsable input is returned as is.
func (m pageRemap) remapComicInfo(existing []byte) []byte {
	if len(existing) == 0 {
		return existing
	}
	var ci comicInfoXML
	if err := xmlUnmarshal(existing, &ci); err != nil {
		return existing
	}
	kept := ci.Pages.Page[:0:0]
	for _, p := range ci.Pages.Page {
		n, ok := m.newIndex[p.Image]
		if !ok {
			continue
		}
		p.Image = n
		kept = append(kept, p)
	}
	ci.Pages.Page = kept
	out, err := xmlMarshalIndent(ci)
	if err != nil {
		return existing
	}
	return out
}

// PageName is the archive entry name for new page index i of total.
func PageName(i, total int, ext string) string {
	width := len(fmt.Sprint(total - 1))
	if width < 3 {
		width = 3
	}
	return fmt.Sprintf("%0*d%s", width, i, strings.ToLower(ext))
}

// ManagePages rewrites book's CBZ so its pages follow plan. See the design
// record above. It never touches the library record; apply PagesResult and
// MarkWritten, then persist.
func ManagePages(book *library.ComicBook, resolvePath func(string) string, tr *trash.Trash, plan PagePlan, opts Options) (PagesResult, error) {
	if resolvePath == nil {
		resolvePath = func(p string) string { return p }
	}
	if !isZipFile(book.FilePath) {
		return PagesResult{}, ErrNotCBZ
	}
	target := resolvePath(book.FilePath)

	zr, err := zip.OpenReader(target)
	if err != nil {
		return PagesResult{}, fmt.Errorf("cbzconvert: read %s: %w", target, err)
	}
	pages := comicvine.SortedZipImageFiles(zr.File)
	oldCount := len(pages)
	zr.Close()
	if oldCount == 0 {
		return PagesResult{}, fmt.Errorf("cbzconvert: no image pages found in %s", target)
	}
	for k, u := range plan.Uploads {
		if !imageExtensions[strings.ToLower(u.Ext)] || len(u.Data) == 0 {
			return PagesResult{}, &PlanError{fmt.Sprintf("upload %d is not an image", k)}
		}
	}
	m, err := buildRemap(plan, oldCount)
	if err != nil {
		return PagesResult{}, err
	}
	if m.unchanged(oldCount) {
		return PagesResult{}, ErrNoPageChange
	}

	res := PagesResult{
		PageCount:           m.count,
		Pages:               m.remapPages(book.Pages, plan.Uploads),
		CurrentPage:         m.pointer(book.CurrentPage),
		LastPageRead:        m.pointer(book.LastPageRead),
		PreferredFrontCover: m.pointer(book.PreferredFrontCover),
		Deleted:             oldCount - len(m.newIndex),
	}
	for _, n := range m.uploads {
		if n >= 0 {
			res.Added++
		}
	}

	updated := *book
	updated.Pages = res.Pages
	updated.PageCount = res.PageCount
	updated.CurrentPage = res.CurrentPage
	updated.LastPageRead = res.LastPageRead
	updated.PreferredFrontCover = res.PreferredFrontCover

	existingInfo, _ := comicvine.ReadComicInfoXMLBytes(target)
	existingInfo = m.remapComicInfo(existingInfo)
	comicInfo, err := buildComicInfoXML(&updated, res.PageCount, nil, existingInfo, false)
	if err != nil {
		return PagesResult{}, fmt.Errorf("cbzconvert: build ComicInfo.xml: %w", err)
	}
	metadata := []metadataEntry{{name: comicInfoEntryName, data: comicInfo}}
	if opts.EmbedComicBook {
		comicBook, err := buildComicBookXML(&updated, res.PageCount, false)
		if err != nil {
			return PagesResult{}, fmt.Errorf("cbzconvert: build ComicBook.xml: %w", err)
		}
		metadata = append(metadata, metadataEntry{name: comicBookEntryName, data: comicBook})
	}

	write := func(tmpPath string) error { return writeManagedCBZ(target, tmpPath, plan, metadata) }
	validate := func(tmpPath string) error { return validateCBZ(tmpPath, res.PageCount, opts.EmbedComicBook) }
	if err := tr.Replace(target, write, validate); err != nil {
		return PagesResult{}, fmt.Errorf("cbzconvert: manage pages of %s: %w", target, err)
	}
	return res, nil
}

// writeManagedCBZ writes dst from src: pages in plan order under new names,
// every other entry copied except the metadata ones that are replaced.
func writeManagedCBZ(src, dst string, plan PagePlan, metadata []metadataEntry) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	pages := comicvine.SortedZipImageFiles(zr.File)

	replace := map[string]bool{}
	for _, m := range metadata {
		replace[strings.ToLower(m.name)] = true
	}
	isPage := make(map[*zip.File]bool, len(pages))
	for _, f := range pages {
		isPage[f] = true
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	for _, f := range zr.File {
		if isPage[f] || replace[strings.ToLower(path.Base(f.Name))] {
			continue
		}
		if err := zw.Copy(f); err != nil {
			zw.Close()
			return err
		}
	}

	now := time.Now()
	total := len(plan.Order)
	for pos, ref := range plan.Order {
		if ref.Orig >= 0 {
			f := pages[ref.Orig]
			hdr := f.FileHeader
			hdr.Name = PageName(pos, total, filepath.Ext(f.Name))
			raw, err := f.OpenRaw()
			if err != nil {
				zw.Close()
				return err
			}
			w, err := zw.CreateRaw(&hdr)
			if err != nil {
				zw.Close()
				return err
			}
			if _, err := io.Copy(w, raw); err != nil {
				zw.Close()
				return err
			}
			continue
		}
		u := plan.Uploads[ref.Upload]
		w, err := zw.CreateHeader(&zip.FileHeader{Name: PageName(pos, total, u.Ext), Method: zip.Store, Modified: now})
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(u.Data); err != nil {
			zw.Close()
			return err
		}
	}

	for _, m := range metadata {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m.name, Method: zip.Deflate, Modified: now})
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(m.data); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}

func xmlUnmarshal(data []byte, v any) error { return xml.Unmarshal(data, v) }

func xmlMarshalIndent(v any) ([]byte, error) {
	out, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}
