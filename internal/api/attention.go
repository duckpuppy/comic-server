package api

import (
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/library"
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
