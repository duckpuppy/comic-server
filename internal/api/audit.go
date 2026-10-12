package api

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// audited returns the library backend wrapped so every update made through
// it is recorded in the audit log under action (comic-server-vqsj). Without
// a config database it is just the plain backend.
func (s *Server) audited(action string) library.Backend {
	if s.configDB == nil {
		return s.backend
	}
	return audit.Wrap(s.backend, action, s.configDB)
}

// AuditEntryJSON is one history entry as returned to clients.
type AuditEntryJSON struct {
	ID          int64          `json:"id"`
	At          string         `json:"at"`
	BookID      string         `json:"book_id"`
	Action      string         `json:"action"`
	Series      string         `json:"series"`
	Number      string         `json:"number"`
	StageBefore string         `json:"stage_before"`
	StageAfter  string         `json:"stage_after"`
	Changes     []audit.Change `json:"changes"`
}

var lastAuditPrune atomic.Int64

// pruneAuditOccasionally trims entries past audit.Retention, at most once an
// hour, piggybacking on reads so it needs no background goroutine.
func (s *Server) pruneAuditOccasionally() {
	now := time.Now()
	last := lastAuditPrune.Load()
	if now.Unix()-last < 3600 || !lastAuditPrune.CompareAndSwap(last, now.Unix()) {
		return
	}
	if n, err := s.configDB.PruneAudit(now.Add(-audit.Retention)); err != nil {
		log.Warn().Err(err).Msg("Failed to prune audit log")
	} else if n > 0 {
		log.Info().Int64("rows", n).Msg("Pruned old audit log entries")
	}
}

// handleAuditList returns audit entries, newest first.
// GET /api/library/audit?book_id=&action=&limit=&offset=
func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	s.pruneAuditOccasionally()

	limit, offset := parseLimitOffset(r, 50, 200)
	rows, total, err := s.configDB.ListAudit(configdb.AuditFilter{
		BookID: r.URL.Query().Get("book_id"),
		Action: r.URL.Query().Get("action"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		http.Error(w, "Failed to load audit log", http.StatusInternalServerError)
		return
	}
	entries := make([]AuditEntryJSON, 0, len(rows))
	for _, row := range rows {
		e := AuditEntryJSON{
			ID: row.ID, At: row.At.Format(time.RFC3339), BookID: row.BookID, Action: row.Action,
			Series: row.Series, Number: row.Number, StageBefore: row.StageBefore, StageAfter: row.StageAfter,
			Changes: []audit.Change{},
		}
		_ = json.Unmarshal([]byte(row.Changes), &e.Changes)
		entries = append(entries, e)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"entries":  entries,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(entries) < total,
	})
}

// RollbackRequest is the body of POST /api/library/audit/rollback.
type RollbackRequest struct {
	BookID string `json:"book_id"`
	Stage  string `json:"stage"`
}

// handleAuditRollback moves one book back to an earlier workflow stage so the
// pipeline re-runs from there. It resets the stage ONLY: field values that
// later stages wrote are not reverted (the audit entries hold the old values
// should a manual revert be wanted), and file moves are not undone. The
// rollback is itself logged.
// POST /api/library/audit/rollback
func (s *Server) handleAuditRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil || s.configDB == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID == "" {
		http.Error(w, "book_id and stage are required", http.StatusBadRequest)
		return
	}
	target := workflow.StageUnknown
	for _, st := range workflow.Stages {
		if st.String() == req.Stage {
			target = st
		}
	}
	if target == workflow.StageUnknown {
		http.Error(w, "Unknown workflow stage", http.StatusBadRequest)
		return
	}

	backend := s.audited(audit.ActionRollback)
	book, err := backend.GetBook(req.BookID)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if !workflow.RegressToStageIfPast(book, target) {
		http.Error(w, "Book is not past that stage; rollback only moves backwards", http.StatusConflict)
		return
	}
	if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
		log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save rollback")
		http.Error(w, "Failed to save book", http.StatusInternalServerError)
		return
	}
	s.InvalidateWorkflowCache()
	s.writeJSON(w, http.StatusOK, map[string]string{"book_id": book.ID, "stage": target.String()})
}
