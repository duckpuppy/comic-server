package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// TestCBLListVisibility covers the end-to-end path comic-server-zw0o added:
// a CBL-imported list (type ComicReadingList) previously had NO browsing
// surface at all - excluded from the tree, the lists browser, and marked
// invisible to handleGetLists. This proves it now shows up everywhere,
// carries its CBL badge flag, and that GetListDetail exposes provenance
// for the reimport UI.
func TestCBLListVisibility(t *testing.T) {
	s, sb := newCBLTestServer(t)

	rl := &cbl.ReadingList{Name: "Atomic Reading Order", Books: []cbl.Book{
		{Series: "Atomic War!", Number: "1", Volume: 1952, Year: 1952},
	}}
	source := storage.CBLImportSource{Source: "git:https://example/repo:list.cbl", SourceRef: "sha1"}
	result, err := sb.DB().ImportCBL(rl, source)
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}

	// 1. handleGetLists (dashboard/browser summary) includes it, flagged.
	req := httptest.NewRequest(http.MethodGet, "/api/library/lists", nil)
	w := httptest.NewRecorder()
	s.handleGetLists(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleGetLists: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listsResp struct {
		Lists []ListSummary `json:"lists"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listsResp); err != nil {
		t.Fatalf("decode handleGetLists response: %v", err)
	}
	found := findSummary(listsResp.Lists, result.ListID)
	if found == nil {
		t.Fatalf("CBL-imported reading list not present in handleGetLists response: %+v", listsResp.Lists)
	}
	if !found.CBLImported {
		t.Errorf("ListSummary.CBLImported = false, want true")
	}
	if found.Type != "ComicReadingList" {
		t.Errorf("ListSummary.Type = %q, want ComicReadingList", found.Type)
	}

	// 2. handleGetListTree includes it too, same flag.
	req = httptest.NewRequest(http.MethodGet, "/api/library/lists/tree", nil)
	w = httptest.NewRecorder()
	s.handleGetListTree(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleGetListTree: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var treeResp struct {
		Tree []ListTreeNode `json:"tree"`
	}
	if err := json.NewDecoder(w.Body).Decode(&treeResp); err != nil {
		t.Fatalf("decode handleGetListTree response: %v", err)
	}
	var treeNode *ListTreeNode
	for i := range treeResp.Tree {
		if treeResp.Tree[i].ID == result.ListID {
			treeNode = &treeResp.Tree[i]
		}
	}
	if treeNode == nil {
		t.Fatalf("CBL-imported reading list not present in tree response: %+v", treeResp.Tree)
	}
	if !treeNode.CBLImported {
		t.Errorf("ListTreeNode.CBLImported = false, want true")
	}

	// 3. handleGetListDetail exposes provenance for the reimport UI.
	req = httptest.NewRequest(http.MethodGet, "/api/library/lists/"+result.ListID, nil)
	w = httptest.NewRecorder()
	s.handleGetListDetail(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleGetListDetail: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var detail ListDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("decode handleGetListDetail response: %v", err)
	}
	if !detail.CBLImported {
		t.Errorf("ListDetail.CBLImported = false, want true")
	}
	if detail.CBLSource != "git:https://example/repo:list.cbl" {
		t.Errorf("ListDetail.CBLSource = %q, want git:https://example/repo:list.cbl", detail.CBLSource)
	}
}

func findSummary(lists []ListSummary, id string) *ListSummary {
	for i := range lists {
		if lists[i].ID == id {
			return &lists[i]
		}
	}
	return nil
}
