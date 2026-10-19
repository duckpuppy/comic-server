package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Page type and bookmark editing (tracker #12). Page Manager already
// reorders, deletes and adds pages in the file; this edits what the library
// says about each page (FrontCover, Advertisement, ... and a bookmark text).
// It changes the library record only and marks the file's ComicInfo.xml out of
// date, so it works for any archive format and the file is rewritten by the
// existing Write metadata action (book page, Out-of-date Files card).

// pageTypes are the values a page may be given. "" clears the type, which
// ComicRack treats as unset. "Deleted" keeps the page in the file but makes
// Convert to CBZ drop it, so it is a different thing from Page Manager's
// Delete.
var pageTypes = []string{
	"", library.PageTypeFrontCover, library.PageTypeBackCover, library.PageTypeStory,
	library.PageTypeAdvertisement, library.PageTypeEditorial, library.PageTypeRoundUp,
	library.PageTypeOther, library.PageTypeDeleted,
}

const pageBookmarkMax = 200

// PageMetaEdit is the new type and bookmark of one page.
type PageMetaEdit struct {
	Index    int    `json:"index"`
	Type     string `json:"type"`
	Bookmark string `json:"bookmark"`
}

// PageMetaRequest is the body of POST /api/library/books/:id/pages/meta.
type PageMetaRequest struct {
	// Version is the file version from the page list; a different one means
	// the pages moved since the person looked, so the indexes may be wrong.
	Version string         `json:"version"`
	Pages   []PageMetaEdit `json:"pages"`
}

// PageMetaResult is the response.
type PageMetaResult struct {
	Changed int `json:"changed"`
}

// pagesMeta applies type/bookmark edits to the book's library page records.
// POST /api/library/books/:id/pages/meta
func (s *Server) pagesMeta(w http.ResponseWriter, r *http.Request, book *library.ComicBook) {
	var req PageMetaRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || len(req.Pages) == 0 {
		http.Error(w, "pages is required and must list at least one page", http.StatusBadRequest)
		return
	}
	backend := s.audited(audit.ActionPageManager)
	working, err := backend.GetBook(book.ID)
	if err != nil || working == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	path := s.resolveBookFilePath(working.FilePath)
	if req.Version != "" && req.Version != fileVersion(path) {
		http.Error(w, "The file changed since the page list was loaded; reload and try again", http.StatusConflict)
		return
	}
	names, _, err := s.pageNames(working)
	if err != nil {
		http.Error(w, "Could not open the archive", http.StatusNotFound)
		return
	}

	// Work on a private copy with its own Pages array. s.audited hands back
	// the live record when no audit log is configured, and its copy is
	// shallow when one is, so a refused request must not be able to change
	// the stored book.
	cp := *working
	working = &cp
	working.Pages = slices.Clone(working.Pages)
	byImage := map[int]int{} // page index -> position in working.Pages
	for i, p := range working.Pages {
		byImage[p.Image] = i
	}
	changed := 0
	seen := map[int]bool{}
	for _, e := range req.Pages {
		if e.Index < 0 || e.Index >= len(names) {
			http.Error(w, fmt.Sprintf("page %d does not exist (the file has %d pages)", e.Index, len(names)), http.StatusBadRequest)
			return
		}
		if seen[e.Index] {
			http.Error(w, fmt.Sprintf("page %d is listed twice", e.Index), http.StatusBadRequest)
			return
		}
		seen[e.Index] = true
		if !slices.Contains(pageTypes, e.Type) {
			http.Error(w, fmt.Sprintf("%q is not a page type", e.Type), http.StatusBadRequest)
			return
		}
		if len([]rune(e.Bookmark)) > pageBookmarkMax {
			http.Error(w, fmt.Sprintf("a bookmark can be at most %d characters", pageBookmarkMax), http.StatusBadRequest)
			return
		}
		pos, ok := byImage[e.Index]
		if !ok {
			if e.Type == "" && e.Bookmark == "" {
				continue // nothing recorded, nothing to clear
			}
			working.Pages = append(working.Pages, library.ComicPageInfo{Image: e.Index})
			pos = len(working.Pages) - 1
			byImage[e.Index] = pos
		}
		pg := &working.Pages[pos]
		if pg.Type == e.Type && pg.Bookmark == e.Bookmark {
			continue
		}
		pg.Type, pg.Bookmark = e.Type, e.Bookmark
		changed++
	}
	if changed == 0 {
		http.Error(w, "Nothing to change", http.StatusConflict)
		return
	}
	sort.SliceStable(working.Pages, func(i, j int) bool { return working.Pages[i].Image < working.Pages[j].Image })
	working.ComicInfoIsDirty = true
	working.ComicBookIsDirty = true
	if err := backend.UpdateBooks([]*library.ComicBook{working}); err != nil {
		log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save page types")
		http.Error(w, "Failed to save book", http.StatusInternalServerError)
		return
	}
	s.InvalidateWorkflowCache()
	s.writeJSON(w, http.StatusOK, PageMetaResult{Changed: changed})
}
