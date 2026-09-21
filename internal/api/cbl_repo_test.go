package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cblrepo"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// newLocalCBLGitFixture creates a real local git repository with .cbl
// files, for testing cbl-repo browse/import without any dependency on
// the real network or the actual DieselTech repo. Mirrors
// internal/cblrepo's own test fixture.
func newLocalCBLGitFixture(t *testing.T) string {
	t.Helper()
	if !cblrepo.GitAvailable() {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	rel := "DC/Batman/Batman No Events.cbl"
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `<ReadingList><Name>Batman No Events</Name><Books>
<Book Series="Atomic War!" Number="1" Volume="1952" Year="1952"/>
</Books></ReadingList>`
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	run("add", "-A")
	run("commit", "-q", "-m", "initial")
	return dir
}

func newCBLRepoTestServer(t *testing.T, withRepo bool) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	t.Cleanup(func() { sb.Close() })

	book := library.ComicBook{ID: "book-1", FilePath: "/x/atomic1.cbz", Series: "Atomic War!", Number: "1", Volume: 1952, Year: 1952}
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{book}}, storage.ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}

	s := &Server{backend: sb, listCache: library.NewListCache(0)}
	if withRepo {
		source := newLocalCBLGitFixture(t)
		clonePath := filepath.Join(t.TempDir(), "clone")
		s.SetCBLRepo(cblrepo.New(source, clonePath))
	}
	return s
}

func TestHandleCBLRepoStatus_NotConfigured(t *testing.T) {
	s := newCBLRepoTestServer(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/library/cbl-repo/status", nil)
	w := httptest.NewRecorder()
	s.handleCBLRepoStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp CBLRepoStatusWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Configured {
		t.Error("expected Configured=false")
	}
}

func TestHandleCBLRepoStatus_ConfiguredNotYetSynced(t *testing.T) {
	s := newCBLRepoTestServer(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/library/cbl-repo/status", nil)
	w := httptest.NewRecorder()
	s.handleCBLRepoStatus(w, req)

	var resp CBLRepoStatusWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Configured {
		t.Error("expected Configured=true")
	}
	if resp.Synced {
		t.Error("expected Synced=false before any browse/sync call")
	}
}

func TestHandleCBLRepoSync_ClonesAndReportsStatus(t *testing.T) {
	s := newCBLRepoTestServer(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/library/cbl-repo/sync", nil)
	w := httptest.NewRecorder()
	s.handleCBLRepoSync(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp CBLRepoStatusWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Synced || resp.FileCount != 1 {
		t.Errorf("expected synced with 1 file, got %+v", resp)
	}
}

func TestHandleCBLRepoBrowse_AutoSyncsOnFirstUse(t *testing.T) {
	s := newCBLRepoTestServer(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/library/cbl-repo/browse", nil)
	w := httptest.NewRecorder()
	s.handleCBLRepoBrowse(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp CBLRepoBrowseResultWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0] != "DC/Batman/Batman No Events.cbl" {
		t.Errorf("Files = %v, want [DC/Batman/Batman No Events.cbl]", resp.Files)
	}
}

func TestHandleCBLRepoBrowse_SearchFilters(t *testing.T) {
	s := newCBLRepoTestServer(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/library/cbl-repo/browse?q=nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleCBLRepoBrowse(w, req)

	var resp CBLRepoBrowseResultWire
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Files) != 0 {
		t.Errorf("expected no matches for an unrelated query, got %v", resp.Files)
	}
}

func TestHandleCBLRepoImport_MatchesAndCreatesList(t *testing.T) {
	s := newCBLRepoTestServer(t, true)

	// Browse first (triggers the initial clone).
	browseReq := httptest.NewRequest(http.MethodGet, "/api/library/cbl-repo/browse", nil)
	browseW := httptest.NewRecorder()
	s.handleCBLRepoBrowse(browseW, browseReq)
	var browseResp CBLRepoBrowseResultWire
	if err := json.NewDecoder(browseW.Body).Decode(&browseResp); err != nil {
		t.Fatalf("decode browse: %v", err)
	}
	if len(browseResp.Files) != 1 {
		t.Fatalf("expected 1 browsable file, got %v", browseResp.Files)
	}

	body, _ := json.Marshal(cblRepoImportRequest{Path: browseResp.Files[0]})
	importReq := httptest.NewRequest(http.MethodPost, "/api/library/cbl-repo/import", bytes.NewReader(body))
	importW := httptest.NewRecorder()
	s.handleCBLRepoImport(importW, importReq)

	if importW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", importW.Code, importW.Body.String())
	}
	var result CBLImportResultWire
	if err := json.NewDecoder(importW.Body).Decode(&result); err != nil {
		t.Fatalf("decode import result: %v", err)
	}
	if result.MatchedOther != 1 || result.Unmatched != 0 {
		t.Errorf("unexpected match result: %+v", result)
	}

	// Provenance recorded with a git: source, not local_file.
	sb := s.backend.(*storage.SQLiteBackend)
	var cblSource, cblSourceRef string
	err := sb.DB().QueryRow("SELECT cbl_source, cbl_source_ref FROM lists WHERE id = ?", result.ListID).Scan(&cblSource, &cblSourceRef)
	if err != nil {
		t.Fatalf("query provenance: %v", err)
	}
	if !strings.HasPrefix(cblSource, "git:") {
		t.Errorf("cbl_source = %q, want a git: prefix", cblSource)
	}
	if cblSourceRef == "" {
		t.Error("expected a non-empty cbl_source_ref (HEAD commit)")
	}
}

func TestHandleCBLRepoImport_RejectsPathTraversal(t *testing.T) {
	s := newCBLRepoTestServer(t, true)
	body, _ := json.Marshal(cblRepoImportRequest{Path: "../../../etc/passwd"})
	req := httptest.NewRequest(http.MethodPost, "/api/library/cbl-repo/import", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCBLRepoImport(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCBLRepoImport_NotConfigured(t *testing.T) {
	s := newCBLRepoTestServer(t, false)
	body, _ := json.Marshal(cblRepoImportRequest{Path: "x.cbl"})
	req := httptest.NewRequest(http.MethodPost, "/api/library/cbl-repo/import", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleCBLRepoImport(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}
