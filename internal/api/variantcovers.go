package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Variant covers (comic-server-0u48): preview-then-apply. ComicVine's
// associated_images carry no marker saying which entries are variant covers
// (image_tags is the catch-all "All Images"), so nothing is added without a
// person picking the images from a preview.

// variantIDsKey records which associated-image ids were already appended,
// '+'-joined (custom values use ',' and '=' as separators). The comic_server_
// prefix keeps it out of the metadata fingerprint.
const variantIDsKey = "comic_server_variant_cover_ids"

// VariantCoverCandidate is one image offered for a book.
type VariantCoverCandidate struct {
	ID      int    `json:"id"`
	URL     string `json:"url"`
	Tags    string `json:"tags,omitempty"`
	Caption string `json:"caption,omitempty"`
	Added   bool   `json:"added"`
}

// VariantCoversInfo is the response of GET /api/library/variant-covers/:id.
type VariantCoversInfo struct {
	BookID     string                  `json:"book_id"`
	Series     string                  `json:"series"`
	Number     string                  `json:"number"`
	IssueID    int                     `json:"issue_id"`
	CanApply   bool                    `json:"can_apply"`
	Reason     string                  `json:"reason,omitempty"` // why CanApply is false
	Candidates []VariantCoverCandidate `json:"candidates"`
}

// VariantCoversApplyRequest is the POST body.
type VariantCoversApplyRequest struct {
	ImageIDs []int `json:"image_ids"`
}

// VariantCoversApplyResult is the POST response.
type VariantCoversApplyResult struct {
	Added     int      `json:"added"`
	Skipped   int      `json:"skipped"` // already in the archive
	Errors    []string `json:"errors,omitempty"`
	PageCount int      `json:"page_count"`
}

func addedVariantIDs(book *library.ComicBook) map[int]bool {
	out := map[int]bool{}
	v, _ := library.GetCustomValue(book.CustomValuesStore, variantIDsKey)
	for _, p := range strings.Split(v, "+") {
		if id, err := strconv.Atoi(p); err == nil {
			out[id] = true
		}
	}
	return out
}

// variantApplyBlocker explains why covers cannot be written for book now, or
// "" when they can. It only covers the write side (config, trash, format);
// fetching candidates needs just the ComicVine client.
func (s *Server) variantApplyBlocker(book *library.ComicBook) string {
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()
	// The trash path lives in config.db once migrated (config.yaml's copy is
	// cleared in memory), so read the effective value, not cfg.Server.TrashPath.
	trashPath, _, err := s.effectiveTrashConfig()
	switch {
	case cfg == nil || !cfg.Server.CBZConvert.Enabled:
		return "cbz_convert is not enabled in config (it also enables writing to archives)"
	case err != nil:
		return "trash settings could not be read: " + err.Error()
	case strings.TrimSpace(trashPath) == "":
		return "trash_path is not configured"
	}
	switch strings.ToLower(extOf(book.FilePath)) {
	case ".cbz", ".zip":
		return ""
	}
	return "only CBZ files can be modified; convert this book to CBZ first"
}

func extOf(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 {
		return p[i:]
	}
	return ""
}

// handleVariantCovers serves GET (candidates) and POST (apply).
// /api/library/variant-covers/:bookId
func (s *Server) handleVariantCovers(w http.ResponseWriter, r *http.Request) {
	bookID := parsePathParam(r.URL.Path, "/api/library/variant-covers/")
	if bookID == "" || strings.Contains(bookID, "/") {
		http.NotFound(w, r)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.cvClient == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}
	book, err := s.backend.GetBook(bookID)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	issueID := comicvine.ExtractCVIssueID(book.CustomValuesStore)
	if issueID == 0 {
		http.Error(w, "Book has no ComicVine issue id; scrape it first", http.StatusConflict)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.variantCoversPreview(w, r, book, issueID)
	case http.MethodPost:
		s.variantCoversApply(w, r, book, issueID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) variantCoversPreview(w http.ResponseWriter, r *http.Request, book *library.ComicBook, issueID int) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	images, err := s.cvClient.FetchIssueImages(ctx, issueID)
	if err != nil {
		log.Warn().Err(err).Int("issue_id", issueID).Msg("Variant cover lookup failed")
		http.Error(w, fmt.Sprintf("ComicVine lookup failed: %v", err), http.StatusBadGateway)
		return
	}
	added := addedVariantIDs(book)
	info := VariantCoversInfo{
		BookID: book.ID, Series: book.Series, Number: book.Number, IssueID: issueID,
		Candidates: []VariantCoverCandidate{},
	}
	for _, img := range images.AssociatedImages {
		c := VariantCoverCandidate{ID: img.ID, URL: img.OriginalURL, Tags: img.ImageTags, Added: added[img.ID]}
		if img.Caption != nil {
			c.Caption = *img.Caption
		}
		info.Candidates = append(info.Candidates, c)
	}
	if reason := s.variantApplyBlocker(book); reason != "" {
		info.Reason = reason
	} else {
		info.CanApply = true
	}
	s.writeJSON(w, http.StatusOK, info)
}

func (s *Server) variantCoversApply(w http.ResponseWriter, r *http.Request, book *library.ComicBook, issueID int) {
	var req VariantCoversApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.ImageIDs) == 0 {
		http.Error(w, "image_ids is required", http.StatusBadRequest)
		return
	}
	if reason := s.variantApplyBlocker(book); reason != "" {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}
	tr, err := s.newTrashFromConfig()
	if err != nil {
		log.Error().Err(err).Msg("Invalid trash configuration for variant covers")
		http.Error(w, "Invalid trash configuration", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Re-fetch server-side and select by id: image URLs from the client are
	// never used, so a request cannot make the server download arbitrary
	// URLs.
	images, err := s.cvClient.FetchIssueImages(ctx, issueID)
	if err != nil {
		http.Error(w, fmt.Sprintf("ComicVine lookup failed: %v", err), http.StatusBadGateway)
		return
	}
	byID := map[int]comicvine.AssociatedImage{}
	for _, img := range images.AssociatedImages {
		byID[img.ID] = img
	}

	var result VariantCoversApplyResult
	var covers []cbzconvert.CoverImage
	seen := map[int]bool{}
	for _, id := range req.ImageIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		img, ok := byID[id]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("image %d is not an image of this issue", id))
			continue
		}
		data, ext, err := s.cvClient.DownloadImage(ctx, img.OriginalURL, s.cvImageAnyHost)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("image %d: %v", id, err))
			continue
		}
		covers = append(covers, cbzconvert.CoverImage{ID: id, Ext: ext, Data: data})
	}
	if len(covers) == 0 {
		s.writeJSON(w, http.StatusOK, result)
		return
	}

	s.configMu.RLock()
	embed := s.config != nil && s.config.Server.CBZConvert.EmbedComicBook
	s.configMu.RUnlock()

	backend := s.audited(audit.ActionVariantCovers)
	working, err := backend.GetBook(book.ID)
	if err != nil || working == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	res, err := cbzconvert.AddVariantCovers(working, s.resolveBookFilePath, tr, covers, cbzconvert.Options{EmbedComicBook: embed})
	switch {
	case errors.Is(err, cbzconvert.ErrNothingToAdd):
		result.Skipped = len(covers)
		result.PageCount = working.PageCount
		s.writeJSON(w, http.StatusOK, result)
		return
	case err != nil:
		log.Error().Err(err).Str("book_id", book.ID).Msg("Adding variant covers failed")
		http.Error(w, fmt.Sprintf("Failed to add covers: %v", err), http.StatusInternalServerError)
		return
	}

	// Record what was added so a repeat run and the preview can tell.
	ids := addedVariantIDs(working)
	for _, id := range res.AddedIDs {
		ids[id] = true
	}
	var joined []string
	for id := range ids {
		joined = append(joined, strconv.Itoa(id))
	}
	working.CustomValuesStore = library.SetCustomValue(working.CustomValuesStore, variantIDsKey, strings.Join(joined, "+"))
	working.PageCount = res.PageCount
	working.Pages = append(working.Pages, res.NewPages...)
	cbzconvert.MarkWritten(working, embed)

	if err := backend.UpdateBooks([]*library.ComicBook{working}); err != nil {
		log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save book after adding covers")
		result.Errors = append(result.Errors, "archive updated but library record not saved: "+err.Error())
	}
	s.InvalidateWorkflowCache()

	result.Added = len(res.AddedIDs)
	result.Skipped = len(covers) - len(res.AddedIDs)
	result.PageCount = res.PageCount
	s.writeJSON(w, http.StatusOK, result)
}
