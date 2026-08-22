package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/komga"
	"github.com/duckpuppy/comic-server/internal/library"
)

func newKomgaTargetTestServer(t *testing.T) *Server {
	t.Helper()
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{
				ID:          "list-1",
				Name:        "Currently Reading",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				},
			},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	return &Server{
		backend:    backend,
		config:     &config.Config{},
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
	}
}

func doKomgaRequest(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	w := httptest.NewRecorder()
	s.handleListsRouter(w, req)
	return w
}

func TestKomgaTarget_GetWhenNoneConfigured(t *testing.T) {
	s := newKomgaTargetTestServer(t)

	w := doKomgaRequest(t, s, http.MethodGet, "/api/library/lists/list-1/komga", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp KomgaTargetForListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Target != nil {
		t.Errorf("expected nil target, got %+v", resp.Target)
	}
}

func TestKomgaTarget_CreateGetUpdateDelete(t *testing.T) {
	s := newKomgaTargetTestServer(t)

	// Create
	createResp := doKomgaRequest(t, s, http.MethodPost, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{
		Type:      "collection",
		KomgaName: "Batman Collection",
		Enabled:   true,
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("expected 200 on create, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var created KomgaTargetResponse
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.ListID != "list-1" || created.ListName != "Currently Reading" || created.Type != "collection" || !created.Enabled {
		t.Errorf("unexpected create response: %+v", created)
	}

	// Duplicate create should conflict
	dupResp := doKomgaRequest(t, s, http.MethodPost, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{
		Type: "collection", KomgaName: "Whatever", Enabled: true,
	})
	if dupResp.Code != http.StatusConflict {
		t.Errorf("expected 409 on duplicate create, got %d", dupResp.Code)
	}

	// Get reflects the created target
	getResp := doKomgaRequest(t, s, http.MethodGet, "/api/library/lists/list-1/komga", nil)
	var getBody KomgaTargetForListResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &getBody); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if getBody.Target == nil || getBody.Target.KomgaName != "Batman Collection" {
		t.Fatalf("expected target to be found via GET, got %+v", getBody)
	}

	// Update
	updateResp := doKomgaRequest(t, s, http.MethodPut, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{
		Type:      "readlist",
		KomgaName: "Batman Read List",
		Enabled:   false,
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d: %s", updateResp.Code, updateResp.Body.String())
	}
	var updated KomgaTargetResponse
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal update response: %v", err)
	}
	if updated.Type != "readlist" || updated.KomgaName != "Batman Read List" || updated.Enabled {
		t.Errorf("unexpected update response: %+v", updated)
	}

	// Config on disk reflects the update
	loaded, err := config.Load(s.configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(loaded.Server.Komga.Targets) != 1 || loaded.Server.Komga.Targets[0].KomgaName != "Batman Read List" {
		t.Errorf("expected saved config to reflect update, got %+v", loaded.Server.Komga.Targets)
	}

	// Delete
	deleteResp := doKomgaRequest(t, s, http.MethodDelete, "/api/library/lists/list-1/komga", nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d: %s", deleteResp.Code, deleteResp.Body.String())
	}

	finalGet := doKomgaRequest(t, s, http.MethodGet, "/api/library/lists/list-1/komga", nil)
	var finalBody KomgaTargetForListResponse
	json.Unmarshal(finalGet.Body.Bytes(), &finalBody)
	if finalBody.Target != nil {
		t.Errorf("expected target to be gone after delete, got %+v", finalBody.Target)
	}

	// Update/Delete on a nonexistent target should 404
	if w := doKomgaRequest(t, s, http.MethodPut, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{Type: "collection", KomgaName: "x", Enabled: true}); w.Code != http.StatusNotFound {
		t.Errorf("expected 404 updating nonexistent target, got %d", w.Code)
	}
	if w := doKomgaRequest(t, s, http.MethodDelete, "/api/library/lists/list-1/komga", nil); w.Code != http.StatusNotFound {
		t.Errorf("expected 404 deleting nonexistent target, got %d", w.Code)
	}
}

func TestKomgaTarget_CreateRejectsInvalidInput(t *testing.T) {
	s := newKomgaTargetTestServer(t)

	if w := doKomgaRequest(t, s, http.MethodPost, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{Type: "bogus", KomgaName: "x", Enabled: true}); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid type, got %d", w.Code)
	}
	if w := doKomgaRequest(t, s, http.MethodPost, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{Type: "collection", KomgaName: "", Enabled: true}); w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty komga_name, got %d", w.Code)
	}
	if w := doKomgaRequest(t, s, http.MethodPost, "/api/library/lists/does-not-exist/komga", KomgaTargetWriteRequest{Type: "collection", KomgaName: "x", Enabled: true}); w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent list, got %d", w.Code)
	}
}

// TestKomgaTarget_ApplyPushesToLiveSyncer verifies that creating, updating,
// and deleting a target through the API immediately reflects in the live
// komga.Syncer's target set (via SetTargets) - the mechanism that lets the
// web UI manage targets without a restart (comic-server-d3w).
func TestKomgaTarget_ApplyPushesToLiveSyncer(t *testing.T) {
	s := newKomgaTargetTestServer(t)
	backend := s.backend
	syncer := komga.NewSyncer(backend, komga.SyncOptions{})
	s.SetKomgaSyncer(syncer)

	doKomgaRequest(t, s, http.MethodPost, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{
		Type: "collection", KomgaName: "Batman Collection", Enabled: true,
	})
	if got := syncer.Targets(); len(got) != 1 || got[0].ListID != "list-1" {
		t.Fatalf("expected live syncer to have 1 target after create, got %+v", got)
	}

	doKomgaRequest(t, s, http.MethodPut, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{
		Type: "collection", KomgaName: "Batman Collection", Enabled: false,
	})
	if got := syncer.Targets(); len(got) != 0 {
		t.Fatalf("expected live syncer to have 0 targets after disabling, got %+v", got)
	}

	doKomgaRequest(t, s, http.MethodPut, "/api/library/lists/list-1/komga", KomgaTargetWriteRequest{
		Type: "collection", KomgaName: "Batman Collection", Enabled: true,
	})
	if got := syncer.Targets(); len(got) != 1 {
		t.Fatalf("expected live syncer to have 1 target after re-enabling, got %+v", got)
	}

	doKomgaRequest(t, s, http.MethodDelete, "/api/library/lists/list-1/komga", nil)
	if got := syncer.Targets(); len(got) != 0 {
		t.Fatalf("expected live syncer to have 0 targets after delete, got %+v", got)
	}
}
