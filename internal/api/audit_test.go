package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newAuditTestServer(t *testing.T, books ...library.ComicBook) *Server {
	t.Helper()
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &Server{
		backend:  library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil),
		configDB: db,
		config:   &config.Config{},
	}
}

func TestAuditRollback_MovesBackLogsAndRefusesForward(t *testing.T) {
	book := library.ComicBook{ID: "1", Series: "S", FilePath: "a.cbz"}
	workflow.SetStage(&book, workflow.StageToMove)
	s := newAuditTestServer(t, book)

	post := func(stage string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(RollbackRequest{BookID: "1", Stage: stage})
		w := httptest.NewRecorder()
		s.handleAuditRollback(w, httptest.NewRequest(http.MethodPost, "/api/library/audit/rollback", bytes.NewReader(body)))
		return w
	}

	if w := post("scrape"); w.Code != http.StatusOK {
		t.Fatalf("rollback status %d: %s", w.Code, w.Body.String())
	}
	cur, _ := s.backend.GetBook("1")
	if got := workflow.GetStage(cur); got != workflow.StageScrape {
		t.Errorf("stage = %v, want scrape", got)
	}

	// Rolling "back" to a later stage is refused.
	if w := post("to_move"); w.Code != http.StatusConflict {
		t.Errorf("forward rollback status = %d, want 409", w.Code)
	}
	if w := post("nonsense"); w.Code != http.StatusBadRequest {
		t.Errorf("bad stage status = %d, want 400", w.Code)
	}

	// The rollback itself shows up in the history.
	w := httptest.NewRecorder()
	s.handleAuditList(w, httptest.NewRequest(http.MethodGet, "/api/library/audit?book_id=1", nil))
	var out struct {
		Entries []AuditEntryJSON `json:"entries"`
		Total   int              `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || out.Entries[0].Action != audit.ActionRollback ||
		out.Entries[0].StageBefore != "to_move" || out.Entries[0].StageAfter != "scrape" {
		t.Errorf("history = %+v", out)
	}
}

func TestAuditedWrite_RecordsFieldChanges(t *testing.T) {
	s := newAuditTestServer(t, library.ComicBook{ID: "1", Series: "S", Title: "Old"})
	b := s.audited(audit.ActionDataManager)
	cur, _ := b.GetBook("1")
	cur.Title = "New"
	if err := b.UpdateBooks([]*library.ComicBook{cur}); err != nil {
		t.Fatal(err)
	}
	rows, total, _ := s.configDB.ListAudit(configdb.AuditFilter{Action: audit.ActionDataManager})
	if total != 1 || rows[0].BookID != "1" {
		t.Fatalf("rows = %+v", rows)
	}
	var changes []audit.Change
	_ = json.Unmarshal([]byte(rows[0].Changes), &changes)
	if len(changes) != 1 || changes[0].Field != "Title" || changes[0].Old != "Old" || changes[0].New != "New" {
		t.Errorf("changes = %+v", changes)
	}
}
