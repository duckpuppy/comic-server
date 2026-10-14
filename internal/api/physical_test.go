package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func postPhysical(t *testing.T, s *Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	w := httptest.NewRecorder()
	if path == "/api/library/physical" {
		s.handlePhysical(w, req)
	} else {
		s.handlePhysicalImport(w, req)
	}
	return w
}

func TestHandlePhysical_CreatesPaperCopyAndRefusesDuplicate(t *testing.T) {
	s := newWorkflowTestServer(t, nil)
	p := PhysicalBook{Series: "Saga", Number: "1", Volume: 2012, Location: "Longbox 12", Condition: "NM", ComicVineIssue: 42}

	w := postPhysical(t, s, "/api/library/physical", p)
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var created map[string]string
	json.NewDecoder(w.Body).Decode(&created)
	b, _ := s.backend.GetBook(created["id"])
	if b == nil || !library.IsPhysical(b) || b.BookLocation != "Longbox 12" || b.BookCondition != "NM" {
		t.Fatalf("book = %+v", b)
	}
	if v, _ := library.GetCustomValue(b.CustomValuesStore, "comicvine_issue"); v != "42" {
		t.Errorf("comicvine_issue = %q", v)
	}

	p.Number = "001" // same issue, leading zeros
	if w := postPhysical(t, s, "/api/library/physical", p); w.Code != http.StatusConflict {
		t.Errorf("duplicate: got %d", w.Code)
	}
	if w := postPhysical(t, s, "/api/library/physical", PhysicalBook{Number: "1"}); w.Code != http.StatusBadRequest {
		t.Errorf("no series: got %d", w.Code)
	}
}

func TestHandlePhysicalImport_PreviewApplyAndReimport(t *testing.T) {
	// A Wanted placeholder for the same issue does not count as a paper copy.
	s := newWorkflowTestServer(t, []library.ComicBook{{ID: "w", Series: "Saga", Number: "2", Volume: 2012}})
	csv := "\uFEFFSERIES,Issue #,vol,Storage Location,Grade,Unknown Col\n" +
		"Saga,1,2012,Box 1,VF,x\n" +
		"Saga,#2,2012,Box 1,,\n" +
		",,,,,\n" +
		"Saga,1,2012,Box 2,,\n" +
		"Saga,3,twenty,Box 1,,\n" +
		",4,,Box 1,,\n"

	w := postPhysical(t, s, "/api/library/physical/import", PhysicalImportRequest{CSV: csv})
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var res PhysicalImportResult
	json.NewDecoder(w.Body).Decode(&res)
	if res.New != 2 || res.Exists != 1 || res.Errors != 2 || res.Added != 0 || res.Applied {
		t.Fatalf("preview = %+v", res)
	}
	if res.Columns["Storage Location"] != "location" || res.Columns["Issue #"] != "number" {
		t.Errorf("columns = %v", res.Columns)
	}
	if len(res.Ignored) != 1 || res.Ignored[0] != "Unknown Col" {
		t.Errorf("ignored = %v", res.Ignored)
	}
	if res.Rows[0].Book.Condition != "VF" || res.Rows[1].Book.Number != "2" {
		t.Errorf("rows = %+v", res.Rows)
	}
	all, _ := s.backend.GetAllBooks()
	if len(all) != 1 {
		t.Fatalf("preview wrote books: %d", len(all))
	}

	w = postPhysical(t, s, "/api/library/physical/import", PhysicalImportRequest{CSV: csv, Apply: true})
	json.NewDecoder(w.Body).Decode(&res)
	if res.Added != 2 || !res.Applied {
		t.Fatalf("apply = %+v", res)
	}
	w = postPhysical(t, s, "/api/library/physical/import", PhysicalImportRequest{CSV: csv, Apply: true})
	res = PhysicalImportResult{}
	json.NewDecoder(w.Body).Decode(&res)
	if res.Added != 0 || res.Exists != 3 {
		t.Fatalf("re-import = %+v", res)
	}
}

func TestHandlePhysicalImport_NeedsSeriesColumn(t *testing.T) {
	s := newWorkflowTestServer(t, nil)
	w := postPhysical(t, s, "/api/library/physical/import", PhysicalImportRequest{CSV: "number,year\n1,2000\n"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d", w.Code)
	}
}
