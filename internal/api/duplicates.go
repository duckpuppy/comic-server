package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/duplicates"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Duplicate review (comic-server-as8l).
//
// GET  /api/library/duplicates?covers=1&limit=&offset=  list suspected groups
// POST /api/library/duplicates/resolve                  resolve one group
//
// Files are never deleted: "keep_trash" quarantines the losers through
// internal/trash. Decisions are remembered in config.db so a resolved or
// intentionally-kept group is not flagged again.

// DuplicateBook is one member of a duplicate group.
type DuplicateBook struct {
	ID        string `json:"id"`
	Series    string `json:"series"`
	Number    string `json:"number"`
	Volume    int    `json:"volume,omitempty"`
	Year      int    `json:"year,omitempty"`
	Title     string `json:"title,omitempty"`
	Publisher string `json:"publisher,omitempty"`
	Format    string `json:"format,omitempty"`
	PageCount int    `json:"page_count"`
	FileSize  int64  `json:"file_size"`
	FilePath  string `json:"file_path"`
	CoverURL  string `json:"cover_url"`
}

// DuplicateGroup is one suspected duplicate set.
type DuplicateGroup struct {
	Key     string          `json:"key"`
	Reasons []string        `json:"reasons"`
	Books   []DuplicateBook `json:"books"`
}

// DuplicateResolveRequest is the body of POST .../duplicates/resolve.
type DuplicateResolveRequest struct {
	GroupKey string `json:"group_key"`
	// Action is "keep" (keep KeepID, dismiss the group, touch nothing),
	// "keep_trash" (keep KeepID, quarantine the rest) or "keep_all"
	// (intentional duplicates, dismiss the group).
	Action string `json:"action"`
	KeepID string `json:"keep_id,omitempty"`
	// Covers must match the list request that produced GroupKey, since the
	// cover signal changes which groups exist.
	Covers bool `json:"covers,omitempty"`
}

// DuplicateResolveResult reports what a resolve did.
type DuplicateResolveResult struct {
	Action  string   `json:"action"`
	Trashed []string `json:"trashed,omitempty"` // book IDs whose file was quarantined
	Errors  []string `json:"errors,omitempty"`
}

// coverHashCache memoises cover dHashes across requests, keyed by
// bookID + resolved path so a replaced file is rehashed.
var coverHashCache sync.Map // string -> uint64

func (s *Server) coverHashFor(b *library.ComicBook) (uint64, bool) {
	p := s.resolveBookFilePath(b.FilePath)
	if p == "" {
		return 0, false
	}
	key := b.ID + "|" + p
	if v, ok := coverHashCache.Load(key); ok {
		return v.(uint64), true
	}
	data, err := comicvine.ExtractCover(p)
	if err != nil {
		return 0, false
	}
	h, err := comicvine.ComputeDHash(data)
	if err != nil {
		return 0, false
	}
	coverHashCache.Store(key, uint64(h))
	return uint64(h), true
}

// detectDuplicateGroups runs detection and drops resolved books and
// remembered keep-all groups.
func (s *Server) detectDuplicateGroups(covers bool) ([]duplicates.Group, error) {
	all, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	var resolved, keepAll map[string]bool
	if s.configDB != nil {
		if resolved, err = s.configDB.ResolvedDuplicateBooks(); err != nil {
			return nil, err
		}
		if keepAll, err = s.configDB.KeepAllDuplicateGroups(); err != nil {
			return nil, err
		}
	}
	books := make([]*library.ComicBook, 0, len(all))
	for i := range all {
		if resolved[all[i].ID] {
			continue
		}
		books = append(books, &all[i])
	}
	opt := duplicates.Options{}
	if covers {
		opt.CoverHash = s.coverHashFor
	}
	groups := duplicates.Detect(books, opt)
	out := groups[:0]
	for _, g := range groups {
		if !keepAll[g.Key] {
			out = append(out, g)
		}
	}
	return out, nil
}

// handleListDuplicates GET /api/library/duplicates
func (s *Server) handleListDuplicates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	covers := r.URL.Query().Get("covers") == "1" || r.URL.Query().Get("covers") == "true"
	groups, err := s.detectDuplicateGroups(covers)
	if err != nil {
		log.Error().Err(err).Msg("Failed to detect duplicates")
		http.Error(w, "Failed to detect duplicates", http.StatusInternalServerError)
		return
	}

	limit, offset := parseLimitOffset(r, 20, 100)
	total := len(groups)
	start := min(offset, total)
	end := min(start+limit, total)

	resp := make([]DuplicateGroup, 0, end-start)
	for _, g := range groups[start:end] {
		dg := DuplicateGroup{Key: g.Key, Reasons: []string{}, Books: []DuplicateBook{}}
		for _, rs := range g.Reasons {
			dg.Reasons = append(dg.Reasons, string(rs))
		}
		for _, b := range g.Books {
			dg.Books = append(dg.Books, DuplicateBook{
				ID: b.ID, Series: b.Series, Number: b.Number, Volume: b.Volume,
				Year: b.Year, Title: b.Title, Publisher: b.Publisher, Format: b.Format,
				PageCount: b.PageCount, FileSize: b.FileSize, FilePath: b.FilePath,
				CoverURL: "/api/library/books/" + b.ID + "/cover",
			})
		}
		resp = append(resp, dg)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"groups":   resp,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": end < total,
		"covers":   covers,
	})
}

// handleResolveDuplicate POST /api/library/duplicates/resolve
func (s *Server) handleResolveDuplicate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	var req DuplicateResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.GroupKey == "" {
		http.Error(w, "group_key is required", http.StatusBadRequest)
		return
	}
	switch req.Action {
	case "keep", "keep_trash":
		if req.KeepID == "" {
			http.Error(w, "keep_id is required", http.StatusBadRequest)
			return
		}
	case "keep_all":
	default:
		http.Error(w, "action must be keep, keep_trash or keep_all", http.StatusBadRequest)
		return
	}

	// Never trust the client's idea of the group: re-detect and match the
	// key, so only books currently flagged together can be trashed.
	groups, err := s.detectDuplicateGroups(req.Covers)
	if err != nil {
		http.Error(w, "Failed to detect duplicates", http.StatusInternalServerError)
		return
	}
	var group *duplicates.Group
	for i := range groups {
		if groups[i].Key == req.GroupKey {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		http.Error(w, "Duplicate group not found (library changed?) - refresh and retry", http.StatusConflict)
		return
	}

	var keeper *library.ComicBook
	if req.Action != "keep_all" {
		for _, b := range group.Books {
			if b.ID == req.KeepID {
				keeper = b
			}
		}
		if keeper == nil {
			http.Error(w, "keep_id is not a member of the group", http.StatusBadRequest)
			return
		}
	}

	result := DuplicateResolveResult{Action: req.Action}

	if req.Action == "keep_trash" {
		tr, err := s.newTrashFromConfig()
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errTrashNotConfigured) {
				status = http.StatusServiceUnavailable
			}
			http.Error(w, "Trash not available: "+err.Error(), status)
			return
		}
		keeperPath := filepath.Clean(s.resolveBookFilePath(keeper.FilePath))
		for _, b := range group.Books {
			if b.ID == keeper.ID {
				continue
			}
			p := s.resolveBookFilePath(b.FilePath)
			if p == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: no file path", b.ID))
				continue
			}
			// Two records pointing at one file: quarantining would
			// destroy the kept book's file too.
			if filepath.Clean(p) == keeperPath {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: shares its file with the kept book, not trashed", b.ID))
				continue
			}
			if _, statErr := os.Stat(p); statErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", b.ID, statErr))
				continue
			}
			if err := tr.Quarantine(p); err != nil {
				log.Error().Err(err).Str("book_id", b.ID).Str("path", p).Msg("Failed to quarantine duplicate")
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", b.ID, err))
				continue
			}
			if err := s.configDB.MarkDuplicateBookResolved(b.ID, keeper.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: file quarantined but decision not recorded: %v", b.ID, err))
			}
			changes, _ := json.Marshal([]audit.Change{
				{Field: "file", Old: b.FilePath, New: "(quarantined)"},
				{Field: "duplicate_of", Old: "", New: keeper.ID},
			})
			audit.Record(s.configDB, configdb.AuditRow{
				BookID: b.ID, Action: audit.ActionDuplicateTrash, Series: b.Series, Number: b.Number,
				Changes: string(changes),
			})
			// Drop the record too: the file is gone, and a record that
			// points at a quarantined file shows up as missing in lists
			// and sync (comic-server-36qq).
			if err := s.backend.DeleteBook(b.ID); err != nil {
				log.Error().Err(err).Str("book_id", b.ID).Msg("Failed to remove trashed duplicate's record")
				result.Errors = append(result.Errors, fmt.Sprintf("%s: file quarantined but record not removed: %v", b.ID, err))
			}
			result.Trashed = append(result.Trashed, b.ID)
		}
		s.InvalidateWorkflowCache()
		// Losers that could not be trashed leave the group flagged; do not
		// record a dismissal for it.
		if len(result.Errors) > 0 {
			s.writeJSON(w, http.StatusOK, result)
			return
		}
	}

	if err := s.configDB.MarkDuplicateGroupKeepAll(group.Key); err != nil {
		http.Error(w, "Failed to record decision", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}
