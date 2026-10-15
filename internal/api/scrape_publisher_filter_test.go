package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

func filterRequest(t *testing.T, s *Server, method string, body any) ScrapePublisherFilterResponse {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/api/settings/scrape-publisher-filter", rdr)
	w := httptest.NewRecorder()
	s.handleScrapePublisherFilter(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", method, w.Code, w.Body.String())
	}
	var got ScrapePublisherFilterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestScrapePublisherFilter_FallsBackThenSavedWins(t *testing.T) {
	s := &Server{config: &config.Config{}, configDB: newTestConfigDB(t)}
	s.config.Server.ScrapePublisherBlock = []string{"Panini"}

	got := filterRequest(t, s, http.MethodGet, nil)
	if got.Source != "config" || !reflect.DeepEqual(got.Block, []string{"Panini"}) {
		t.Fatalf("fallback = %+v", got)
	}

	// Blanks and case-duplicates are cleaned; saved value overrides config.
	got = filterRequest(t, s, http.MethodPut, ScrapePublisherFilterResponse{
		Allow: []string{" DC ", "dc", "", "Marvel"}, Block: []string{},
	})
	if got.Source != "saved" || !reflect.DeepEqual(got.Allow, []string{"DC", "Marvel"}) {
		t.Fatalf("put = %+v", got)
	}

	f, source := s.effectiveScrapePublisherFilter()
	if source != "saved" || len(f.Block) != 0 || len(f.Allow) != 2 {
		t.Errorf("effective = %+v (%s); saved empty block must override config", f, source)
	}
}

func TestScrapePublisherFilter_PutRejectsOversize(t *testing.T) {
	s := &Server{config: &config.Config{}, configDB: newTestConfigDB(t)}
	long := make([]byte, maxPublisherFilterTermLen+1)
	for i := range long {
		long[i] = 'a'
	}
	b, _ := json.Marshal(ScrapePublisherFilterResponse{Block: []string{string(long)}})
	w := httptest.NewRecorder()
	s.handleScrapePublisherFilter(w, httptest.NewRequest(http.MethodPut, "/x", bytes.NewReader(b)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}
