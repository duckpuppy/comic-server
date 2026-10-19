package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/covers"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Page manager (comic-server-j1cv). GET lists a CBZ's pages, GET .../:n serves
// one page as a thumbnail, POST applies a reorder / delete / add plan in one
// atomic write (cbzconvert.ManagePages). Nothing is staged on the server: the
// page stages the plan client-side and sends it once.

const (
	pageUploadMaxBytes  = 25 << 20  // per uploaded page
	pagePlanMaxBytes    = 200 << 20 // whole request
	pageThumbDefault    = 240
	pageThumbMax        = 1600
	pageUploadMaxPieces = 200
)

// BookPage is one page in the listing.
type BookPage struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Type     string `json:"type,omitempty"`
	Bookmark string `json:"bookmark,omitempty"`
}

// BookPagesInfo is the response of GET /api/library/books/:id/pages.
type BookPagesInfo struct {
	BookID  string `json:"book_id"`
	Series  string `json:"series"`
	Number  string `json:"number"`
	Version string `json:"version"` // changes whenever the file does; echoed back by POST
	CanEdit bool   `json:"can_edit"`
	// PageTypes are the values the type menu offers ("" = none).
	PageTypes []string   `json:"page_types"`
	Reason    string     `json:"reason,omitempty"` // why CanEdit is false
	Pages     []BookPage `json:"pages"`
}

// pagePlanRequest is the JSON `plan` field of the POST.
type pagePlanRequest struct {
	Version string `json:"version"`
	Order   []struct {
		Page   *int `json:"page,omitempty"`   // current page index
		Upload *int `json:"upload,omitempty"` // index among the uploaded files
	} `json:"order"`
}

// PageManagerResult is the POST response.
type PageManagerResult struct {
	PageCount int    `json:"page_count"`
	Deleted   int    `json:"deleted"`
	Added     int    `json:"added"`
	Version   string `json:"version"`
	Warning   string `json:"warning,omitempty"`
}

func fileVersion(p string) string {
	info, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
}

// handleBookPages routes /api/library/books/:id/pages and .../pages/:n.
func (s *Server) handleBookPages(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/library/books/")
	bookID, rest, _ := strings.Cut(suffix, "/pages")
	rest = strings.Trim(rest, "/")
	if bookID == "" || strings.Contains(bookID, "/") {
		http.NotFound(w, r)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	book, err := s.backend.GetBook(bookID)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if book.FilePath == "" {
		http.Error(w, "Book has no file", http.StatusNotFound)
		return
	}

	switch {
	case rest == "" && r.Method == http.MethodGet:
		s.pagesList(w, book)
	case rest == "" && r.Method == http.MethodPost:
		s.pagesApply(w, r, book)
	case rest == "meta" && r.Method == http.MethodPost:
		s.pagesMeta(w, r, book)
	case rest != "" && r.Method == http.MethodGet:
		n, err := strconv.Atoi(rest)
		if err != nil || n < 0 {
			http.NotFound(w, r)
			return
		}
		s.pageImage(w, r, book, n)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// pageManagerBlocker explains why pages cannot be edited, or "".
func (s *Server) pageManagerBlocker(book *library.ComicBook) string {
	return s.variantApplyBlocker(book)
}

func zipPageFiles(zr *zip.ReadCloser) []*zip.File { return comicvine.SortedZipImageFiles(zr.File) }

// pageNames lists the book's page entry names (and sizes for a CBZ), in page order.
func (s *Server) pageNames(book *library.ComicBook) (names []string, sizes []int64, err error) {
	path := s.resolveBookFilePath(book.FilePath)
	if cbzconvert.IsZipPath(book.FilePath) {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, nil, err
		}
		defer zr.Close()
		for _, f := range zipPageFiles(zr) {
			names = append(names, f.Name)
			sizes = append(sizes, int64(f.UncompressedSize64))
		}
		return names, sizes, nil
	}
	// CBR/CB7: names only (the reader needs the count, not sizes).
	names, err = comicvine.ListPageNames(path)
	return names, nil, err
}

func (s *Server) pagesList(w http.ResponseWriter, book *library.ComicBook) {
	info := BookPagesInfo{BookID: book.ID, Series: book.Series, Number: book.Number, Pages: []BookPage{}, PageTypes: pageTypes}
	path := s.resolveBookFilePath(book.FilePath)
	names, sizes, err := s.pageNames(book)
	if err != nil {
		log.Warn().Err(err).Str("book_id", book.ID).Msg("Page list could not open archive")
		http.Error(w, "Could not open the archive", http.StatusNotFound)
		return
	}
	info.Version = fileVersion(path)
	byImage := map[int]library.ComicPageInfo{}
	for _, p := range book.Pages {
		byImage[p.Image] = p
	}
	for i, name := range names {
		pg := BookPage{Index: i, Name: name}
		if i < len(sizes) {
			pg.Size = sizes[i]
		}
		if lp, ok := byImage[i]; ok {
			pg.Type, pg.Bookmark = lp.Type, lp.Bookmark
		}
		info.Pages = append(info.Pages, pg)
	}
	switch {
	case !cbzconvert.IsZipPath(book.FilePath):
		info.Reason = "only CBZ files can be modified; convert this book to CBZ first"
	default:
		if reason := s.pageManagerBlocker(book); reason != "" {
			info.Reason = reason
		} else {
			info.CanEdit = true
		}
	}
	s.writeJSON(w, http.StatusOK, info)
}

// pageImage serves page n. Default is a JPEG thumbnail (?w= pixels wide); with
// ?full=1 it is the page's original bytes, for the reader (comic-server-65o8).
func (s *Server) pageImage(w http.ResponseWriter, r *http.Request, book *library.ComicBook, n int) {
	data, err := comicvine.ReadPage(s.resolveBookFilePath(book.FilePath), n)
	if err != nil {
		if strings.Contains(err.Error(), "out of range") {
			http.NotFound(w, r)
			return
		}
		log.Warn().Err(err).Str("book_id", book.ID).Int("page", n).Msg("Could not read page")
		http.Error(w, "Could not read the page", http.StatusInternalServerError)
		return
	}
	// The URL carries ?v=<version>, so a changed file gets a new URL.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if r.URL.Query().Get("full") == "1" {
		ct := http.DetectContentType(data)
		if !strings.HasPrefix(ct, "image/") {
			http.Error(w, "Page is not a readable image", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Write(data)
		return
	}

	width := pageThumbDefault
	if v, err := strconv.Atoi(r.URL.Query().Get("w")); err == nil && v >= 50 && v <= pageThumbMax {
		width = v
	}
	thumb, err := covers.Thumbnail(data, width)
	if err != nil {
		http.Error(w, "Page is not a readable image", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(thumb)
}

func (s *Server) pagesApply(w http.ResponseWriter, r *http.Request, book *library.ComicBook) {
	if reason := s.pageManagerBlocker(book); reason != "" {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, pagePlanMaxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Invalid form: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	var req pagePlanRequest
	if err := json.Unmarshal([]byte(r.FormValue("plan")), &req); err != nil || len(req.Order) == 0 {
		http.Error(w, "plan is required and must list at least one page", http.StatusBadRequest)
		return
	}
	headers := r.MultipartForm.File["upload"]
	if len(headers) > pageUploadMaxPieces {
		http.Error(w, "too many uploaded pages", http.StatusBadRequest)
		return
	}
	plan := cbzconvert.PagePlan{}
	for _, h := range headers {
		if h.Size > pageUploadMaxBytes {
			http.Error(w, fmt.Sprintf("%s is larger than %d MB", h.Filename, pageUploadMaxBytes>>20), http.StatusRequestEntityTooLarge)
			return
		}
		f, err := h.Open()
		if err != nil {
			http.Error(w, "Could not read an uploaded page", http.StatusBadRequest)
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, pageUploadMaxBytes+1))
		f.Close()
		if err != nil {
			http.Error(w, "Could not read an uploaded page", http.StatusBadRequest)
			return
		}
		// Sniff the content: the extension comes from what the bytes are, not
		// from the client's file name.
		ext := ""
		switch http.DetectContentType(data) {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		}
		if ext == "" {
			http.Error(w, fmt.Sprintf("%s is not a JPEG, PNG, GIF or WebP image", filepath.Base(h.Filename)), http.StatusBadRequest)
			return
		}
		plan.Uploads = append(plan.Uploads, cbzconvert.UploadedPage{Ext: ext, Data: data})
	}
	for _, o := range req.Order {
		switch {
		case o.Page != nil && o.Upload == nil:
			plan.Order = append(plan.Order, cbzconvert.PageRef{Orig: *o.Page})
		case o.Upload != nil && o.Page == nil:
			plan.Order = append(plan.Order, cbzconvert.PageRef{Orig: -1, Upload: *o.Upload})
		default:
			http.Error(w, "each order entry needs exactly one of page or upload", http.StatusBadRequest)
			return
		}
	}

	tr, err := s.newTrashFromConfig()
	if err != nil {
		log.Error().Err(err).Msg("Invalid trash configuration for page manager")
		http.Error(w, "Invalid trash configuration", http.StatusInternalServerError)
		return
	}
	s.configMu.RLock()
	embed := s.config != nil && s.config.Server.CBZConvert.EmbedComicBook
	s.configMu.RUnlock()

	backend := s.audited(audit.ActionPageManager)
	working, err := backend.GetBook(book.ID)
	if err != nil || working == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	path := s.resolveBookFilePath(working.FilePath)
	// The plan refers to page indexes of the file the person was looking at.
	if req.Version != "" && req.Version != fileVersion(path) {
		http.Error(w, "The file changed since the page list was loaded; reload and try again", http.StatusConflict)
		return
	}

	res, err := cbzconvert.ManagePages(working, s.resolveBookFilePath, tr, plan, cbzconvert.Options{EmbedComicBook: embed})
	var planErr *cbzconvert.PlanError
	switch {
	case errors.Is(err, cbzconvert.ErrNoPageChange):
		http.Error(w, "Nothing to change", http.StatusConflict)
		return
	case errors.As(err, &planErr):
		http.Error(w, planErr.Error(), http.StatusBadRequest)
		return
	case err != nil:
		log.Error().Err(err).Str("book_id", book.ID).Msg("Page manager write failed")
		http.Error(w, fmt.Sprintf("Failed to update pages: %v", err), http.StatusInternalServerError)
		return
	}

	working.Pages = res.Pages
	working.PageCount = res.PageCount
	working.CurrentPage = res.CurrentPage
	working.LastPageRead = res.LastPageRead
	working.PreferredFrontCover = res.PreferredFrontCover
	cbzconvert.MarkWritten(working, embed)

	out := PageManagerResult{PageCount: res.PageCount, Deleted: res.Deleted, Added: res.Added, Version: fileVersion(path)}
	if err := backend.UpdateBooks([]*library.ComicBook{working}); err != nil {
		log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save book after page manager write")
		out.Warning = "archive updated but library record not saved: " + err.Error()
	}
	if s.coverCache != nil {
		_ = s.coverCache.Invalidate(book.ID)
	}
	s.InvalidateWorkflowCache()
	s.writeJSON(w, http.StatusOK, out)
}
