package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// AttentionFlagSummary is one flag's entry in GET /api/library/attention.
type AttentionFlagSummary struct {
	Flag     string `json:"flag"`
	Label    string `json:"label"`
	Hint     string `json:"hint"`
	Severity string `json:"severity"` // error | warn | info
	Count    int    `json:"count"`
}

// handleAttentionSummary returns every flag with a non-zero count, ranked
// by severity then count (comic-server-z77p).
// GET /api/library/attention
func (s *Server) handleAttentionSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	counts := make(map[attention.Flag]int, len(entry.booksByFlag))
	for f, books := range entry.booksByFlag {
		counts[f] = len(books)
	}
	flags := []AttentionFlagSummary{}
	for _, sum := range attention.Summarize(counts) {
		flags = append(flags, AttentionFlagSummary{
			Flag:     string(sum.Flag),
			Label:    sum.Label,
			Hint:     sum.Hint,
			Severity: sum.Severity.String(),
			Count:    sum.Count,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"flags": flags})
}

// cvAttentionTTL bounds how stale the ComicVine side of the attention flags
// can be. The CV sync moves slowly (weeks for a full library), so a few
// minutes of lag costs nothing and keeps the chunked issue lookup off most
// workflow-cache rebuilds.
const cvAttentionTTL = 10 * time.Minute

// cvAttentionData returns the ComicVine cache slice the attention flags
// read, or nil without a cache or on error (the CV flags then just do not
// show). Issue rows are looked up only for issue IDs the library uses. A
// book tagged after the snapshot was taken is unknown until the next one.
func (s *Server) cvAttentionData(books []library.ComicBook) *attention.CVData {
	if s.cvCache == nil {
		return nil
	}
	s.cvAttentionMu.Lock()
	defer s.cvAttentionMu.Unlock()
	if s.cvAttention != nil && time.Since(s.cvAttentionAt) < cvAttentionTTL {
		return s.cvAttention
	}
	counts, err := s.cvCache.SyncedIssueCounts()
	if err != nil {
		log.Warn().Err(err).Msg("Attention: failed to read ComicVine issue counts")
		return nil
	}
	seen := map[int]bool{}
	var ids []int
	for i := range books {
		if _, iss := attention.BookCVIDs(&books[i]); iss > 0 && !seen[iss] {
			seen[iss] = true
			ids = append(ids, iss)
		}
	}
	refs, err := s.cvCache.IssueRefs(ids)
	if err != nil {
		log.Warn().Err(err).Msg("Attention: failed to read ComicVine issues")
		return nil
	}
	issues := make(map[int]attention.CVIssue, len(refs))
	for id, r := range refs {
		issues[id] = attention.CVIssue{Volume: r.Volume, Number: r.Number}
	}
	s.cvAttention = &attention.CVData{Counts: counts, Issues: issues}
	s.cvAttentionAt = time.Now()
	return s.cvAttention
}

// handleAttentionFlagBooks returns a page of the books carrying one flag.
// GET /api/library/attention/:flag/books
func (s *Server) handleAttentionFlagBooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/library/attention/"), "/books")
	flag := attention.Flag(name)
	if _, ok := attention.Lookup(flag); !ok || !strings.HasSuffix(r.URL.Path, "/books") {
		http.NotFound(w, r)
		return
	}
	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	matched := entry.booksByFlag[flag]

	limit, offset := parseLimitOffset(r, 20, 100)
	total := len(matched)
	start := min(offset, total)
	end := min(start+limit, total)

	previews := make([]ComicPreview, 0, end-start)
	for _, book := range matched[start:end] {
		previews = append(previews, attentionPreview(book))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"comics":   previews,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
	})
}

func attentionPreview(book *library.ComicBook) ComicPreview {
	return ComicPreview{
		ID:          book.ID,
		Series:      book.Series,
		Number:      book.Number,
		Title:       book.Title,
		Volume:      book.Volume,
		Publisher:   book.Publisher,
		Year:        book.Year,
		Unread:      book.IsUnread(),
		CurrentPath: book.FilePath,
	}
}
