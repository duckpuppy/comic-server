package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

func newDuplicatesTestServer(t *testing.T, trashPath string, books []library.ComicBook) *Server {
	t.Helper()
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, trashPath, books)
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s.configDB = db
	return s
}

func listDuplicates(t *testing.T, s *Server) []DuplicateGroup {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleListDuplicates(w, httptest.NewRequest(http.MethodGet, "/api/library/duplicates", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Groups []DuplicateGroup }
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp.Groups
}

func resolveDuplicate(t *testing.T, s *Server, req DuplicateResolveRequest) (*httptest.ResponseRecorder, DuplicateResolveResult) {
	t.Helper()
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	s.handleResolveDuplicate(w, httptest.NewRequest(http.MethodPost, "/api/library/duplicates/resolve", bytes.NewReader(body)))
	var res DuplicateResolveResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	return w, res
}

func twoDupBooks(t *testing.T) (a, b string, books []library.ComicBook) {
	dir := t.TempDir()
	a = filepath.Join(dir, "a.cbz")
	b = filepath.Join(dir, "b.cbz")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	books = []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "001", FilePath: a, PageCount: 22},
		{ID: "2", Series: "Batman", Number: "1", FilePath: b, PageCount: 24},
		{ID: "3", Series: "Batman", Number: "2", FilePath: filepath.Join(dir, "c.cbz"), PageCount: 24},
	}
	return
}

func TestDuplicates_KeepTrashQuarantinesLosersAndRemembers(t *testing.T) {
	a, b, books := twoDupBooks(t)
	trashDir := t.TempDir()
	s := newDuplicatesTestServer(t, trashDir, books)

	groups := listDuplicates(t, s)
	if len(groups) != 1 || len(groups[0].Books) != 2 {
		t.Fatalf("groups = %+v", groups)
	}

	w, res := resolveDuplicate(t, s, DuplicateResolveRequest{GroupKey: groups[0].Key, Action: "keep_trash", KeepID: "1"})
	if w.Code != http.StatusOK || len(res.Errors) != 0 || len(res.Trashed) != 1 || res.Trashed[0] != "2" {
		t.Fatalf("resolve = %d %+v", w.Code, res)
	}
	// The loser's record goes with its file (comic-server-36qq); the kept
	// book is untouched.
	if got, _ := s.backend.GetBook("2"); got != nil {
		t.Error("trashed loser's library record should be removed")
	}
	if got, _ := s.backend.GetBook("1"); got == nil {
		t.Error("kept book's record must remain")
	}
	if _, err := os.Stat(a); err != nil {
		t.Errorf("kept file missing: %v", err)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Errorf("loser file should be quarantined, stat err = %v", err)
	}
	// Quarantined, not deleted: something exists under the trash root.
	var found bool
	_ = filepath.Walk(trashDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("no file in trash dir")
	}
	if g := listDuplicates(t, s); len(g) != 0 {
		t.Errorf("group should not be re-flagged, got %+v", g)
	}
}

func TestDuplicates_KeepAllPersistsUntilGroupChanges(t *testing.T) {
	_, _, books := twoDupBooks(t)
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	groups := listDuplicates(t, s)
	w, _ := resolveDuplicate(t, s, DuplicateResolveRequest{GroupKey: groups[0].Key, Action: "keep_all"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if g := listDuplicates(t, s); len(g) != 0 {
		t.Fatalf("keep-all group re-flagged: %+v", g)
	}
	// A third copy joins: membership changes, so it is flagged again.
	if err := s.backend.CreateBook(&library.ComicBook{ID: "9", Series: "Batman", Number: "1", FilePath: "/z.cbz", PageCount: 22}); err != nil {
		t.Fatal(err)
	}
	if g := listDuplicates(t, s); len(g) != 1 || len(g[0].Books) != 3 {
		t.Fatalf("expected re-flag with 3 books, got %+v", g)
	}
}

func TestDuplicates_ResolveRejectsUnknownGroupAndBadKeeper(t *testing.T) {
	_, _, books := twoDupBooks(t)
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	groups := listDuplicates(t, s)

	if w, _ := resolveDuplicate(t, s, DuplicateResolveRequest{GroupKey: "nope", Action: "keep_trash", KeepID: "1"}); w.Code != http.StatusConflict {
		t.Errorf("unknown group status = %d, want 409", w.Code)
	}
	// Book 3 exists but is not in the group - must not be accepted as keeper.
	if w, _ := resolveDuplicate(t, s, DuplicateResolveRequest{GroupKey: groups[0].Key, Action: "keep_trash", KeepID: "3"}); w.Code != http.StatusBadRequest {
		t.Errorf("non-member keeper status = %d, want 400", w.Code)
	}
}

func TestDuplicates_NeverTrashesFileSharedWithKeeper(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "same.cbz")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	books := []library.ComicBook{
		{ID: "1", Series: "Batman", Number: "1", FilePath: p, PageCount: 22},
		{ID: "2", Series: "Batman", Number: "1", FilePath: p, PageCount: 22},
	}
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	groups := listDuplicates(t, s)
	_, res := resolveDuplicate(t, s, DuplicateResolveRequest{GroupKey: groups[0].Key, Action: "keep_trash", KeepID: "1"})
	if len(res.Trashed) != 0 || len(res.Errors) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("shared file was removed: %v", err)
	}
	// Failure leaves the group flagged.
	if g := listDuplicates(t, s); len(g) != 1 {
		t.Errorf("group should remain, got %+v", g)
	}
}

func TestDuplicates_KeepTrashRequiresTrashConfigured(t *testing.T) {
	_, _, books := twoDupBooks(t)
	s := newDuplicatesTestServer(t, "", books)
	groups := listDuplicates(t, s)
	w, _ := resolveDuplicate(t, s, DuplicateResolveRequest{GroupKey: groups[0].Key, Action: "keep_trash", KeepID: "1"})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
