package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func getJSON(t *testing.T, s *Server, path string, out any) int {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleBooksRouter(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code == http.StatusOK && out != nil {
		if err := json.NewDecoder(w.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code
}

func TestBookDetail(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.cbz")
	writeTestCBZWithComicInfo(t, p, `<?xml version="1.0"?><ComicInfo><Series>Batman</Series><Number>1</Number><Year>1999</Year><Pages><Page Image="0"/></Pages></ComicInfo>`)
	books := []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "1", Year: 2011, PageCount: 2, FilePath: p,
			CustomValuesStore: "comicvine_volume=42,comicvine_issue=7"},
		{ID: "w1", Series: "Batman", Number: "2"},
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)

	var d BookDetail
	if code := getJSON(t, s, "/api/library/books/b1", &d); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if d.Book.Series != "Batman" || d.File == nil || !d.File.Exists || d.File.Ext != ".cbz" {
		t.Fatalf("detail = %+v file = %+v", d.Book, d.File)
	}
	if d.ComicVine == nil || d.ComicVine.VolumeID != 42 || d.ComicVine.IssueURL != "https://comicvine.gamespot.com/issue/4000-7/" {
		t.Fatalf("comicvine = %+v", d.ComicVine)
	}
	// cbz_convert is off and no scraper: actions say why they cannot run.
	if d.Actions.WriteMetadataBlocker == "" || d.Actions.ScrapeBlocker == "" || d.Actions.VariantCoversBlocker == "" {
		t.Errorf("actions = %+v", d.Actions)
	}
	if d.CustomValues["comicvine_volume"] != "42" {
		t.Errorf("custom values = %v", d.CustomValues)
	}
	hasFlag := false
	for _, f := range d.Flags {
		if f.Flag == "missing_format" {
			hasFlag = true
		}
	}
	if !hasFlag {
		t.Errorf("flags = %+v", d.Flags)
	}

	var ci ComicInfoComparison
	if code := getJSON(t, s, "/api/library/books/b1/comicinfo", &ci); code != http.StatusOK {
		t.Fatalf("comicinfo status %d", code)
	}
	if !ci.Present || ci.Raw == "" || ci.Same == 0 {
		t.Fatalf("comicinfo = %+v", ci)
	}
	diff := map[string]ComicInfoField{}
	for _, f := range ci.Diff {
		diff[f.Field] = f
	}
	if diff["Year"].File != "1999" || diff["Year"].Library != "2011" {
		t.Errorf("Year diff = %+v (all %+v)", diff["Year"], ci.Diff)
	}
	if _, ok := diff["Series"]; ok {
		t.Errorf("Series should agree: %+v", ci.Diff)
	}
	if _, ok := diff["Pages"]; ok {
		t.Error("Pages must be skipped")
	}

	// Fileless book: no file block, comicinfo explains.
	d = BookDetail{}
	getJSON(t, s, "/api/library/books/w1", &d)
	if d.File != nil {
		t.Errorf("fileless book has file %+v", d.File)
	}
	ci = ComicInfoComparison{}
	getJSON(t, s, "/api/library/books/w1/comicinfo", &ci)
	if ci.Present || ci.Error == "" {
		t.Errorf("fileless comicinfo = %+v", ci)
	}

	if code := getJSON(t, s, "/api/library/books/nope", nil); code != http.StatusNotFound {
		t.Errorf("missing book status %d", code)
	}
	if code := getJSON(t, s, "/api/library/books/b1/other", nil); code != http.StatusNotFound {
		t.Errorf("unknown suffix status %d", code)
	}
}

func TestBookDetail_ActionsWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.cbz")
	writeTestCBZFixture(t, p)
	b := library.ComicBook{ID: "b1", Series: "Batman", FilePath: p}
	workflow.SetStage(&b, workflow.StageDataManager)
	s := newDuplicatesTestServer(t, t.TempDir(), []library.ComicBook{b})
	s.config.Server.CBZConvert.Enabled = true

	var d BookDetail
	if code := getJSON(t, s, "/api/library/books/b1", &d); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if d.Actions.WriteMetadataBlocker != "" {
		t.Errorf("write blocked: %q", d.Actions.WriteMetadataBlocker)
	}
	// No CV client and no issue tag: variant covers still blocked.
	if d.Actions.VariantCoversBlocker == "" {
		t.Error("variant covers should be blocked without a ComicVine client")
	}
	var got []string
	for _, st := range d.Actions.RollbackStages {
		got = append(got, st.Value)
	}
	if strings.Join(got, ",") != "convert_cbz,scrape,scan_info" {
		t.Errorf("rollback stages = %v", got)
	}
}

func TestAcceptFlag_HidesFlagAndCanUndo(t *testing.T) {
	books := []library.ComicBook{
		{ID: "b1", Series: "Batman", FilePath: "a.cbz"}, // missing_format, missing_count, unknown_publisher
		{ID: "b2", Series: "Batman", FilePath: "b.cbz"},
	}
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	count := func() int {
		w := httptest.NewRecorder()
		s.handleAttentionSummary(w, httptest.NewRequest(http.MethodGet, "/", nil))
		var resp struct{ Flags []AttentionFlagSummary }
		_ = json.NewDecoder(w.Body).Decode(&resp)
		for _, f := range resp.Flags {
			if f.Flag == "missing_format" {
				return f.Count
			}
		}
		return 0
	}
	post := func(body string) int {
		w := httptest.NewRecorder()
		s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/b1/accept-flag", strings.NewReader(body)))
		return w.Code
	}
	if count() != 2 {
		t.Fatalf("before: %d", count())
	}
	if code := post(`{"flag":"missing_format","accepted":true}`); code != http.StatusOK {
		t.Fatalf("accept status %d", code)
	}
	if count() != 1 {
		t.Fatalf("after accept: %d", count())
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/b1", &d)
	if len(d.Accepted) != 1 || d.Accepted[0].Flag != "missing_format" {
		t.Errorf("accepted = %+v", d.Accepted)
	}
	for _, f := range d.Flags {
		if f.Flag == "missing_format" {
			t.Error("accepted flag still listed as active")
		}
	}
	// Audited.
	rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{BookID: "b1", Limit: 10})
	if len(rows) != 1 || rows[0].Action != "accept_flag" {
		t.Errorf("audit rows = %+v", rows)
	}
	if code := post(`{"flag":"missing_format","accepted":false}`); code != http.StatusOK || count() != 2 {
		t.Fatalf("undo: status %d count %d", code, count())
	}
	if code := post(`{"flag":"bogus","accepted":true}`); code != http.StatusBadRequest {
		t.Errorf("unknown flag status %d", code)
	}
}

func TestCVNoMatch_MarksMovesStageAndUndoes(t *testing.T) {
	b1 := library.ComicBook{ID: "b1", Series: "Omnibus", FilePath: "a.cbz", ScanInformation: "s"}
	workflow.SetStage(&b1, workflow.StageScrape)
	books := []library.ComicBook{b1, {ID: "w1", Series: "Wanted"}}
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	post := func(id, body string) int {
		w := httptest.NewRecorder()
		s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/"+id+"/cv-nomatch", strings.NewReader(body)))
		return w.Code
	}
	if c := post("b1", `{"set":true,"reason":"bogus"}`); c != http.StatusBadRequest {
		t.Errorf("bad reason status %d", c)
	}
	if c := post("w1", `{"set":true,"reason":"collected"}`); c != http.StatusConflict {
		t.Errorf("fileless status %d", c)
	}
	if c := post("b1", `{"set":true,"reason":"collected"}`); c != http.StatusOK {
		t.Fatalf("set status %d", c)
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/b1", &d)
	if d.NoCVMatch == nil || d.NoCVMatch.Reason != "collected" {
		t.Fatalf("detail cv_nomatch = %+v", d.NoCVMatch)
	}
	if d.Stage == "scrape" || d.Stage == "" {
		t.Errorf("stage after marking = %q, want past scrape", d.Stage)
	}
	if c := post("b1", `{"set":false}`); c != http.StatusOK {
		t.Fatalf("undo status %d", c)
	}
	d = BookDetail{}
	getJSON(t, s, "/api/library/books/b1", &d)
	if d.NoCVMatch != nil || d.Stage != "scrape" {
		t.Errorf("after undo: %+v stage %q", d.NoCVMatch, d.Stage)
	}
}

func TestMigrateCVDBSkip_CopiesTagIntoMarkerOnce(t *testing.T) {
	stuck := library.ComicBook{ID: "s1", Series: "Cover Pack", FilePath: "a.cbz", Tags: "CVDBSKIP", ScanInformation: "s"}
	workflow.SetStage(&stuck, workflow.StageScrape)
	tagged := library.ComicBook{ID: "t1", Series: "Real", FilePath: "b.cbz", Tags: "CVDBSKIP", CustomValuesStore: ",comicvine_volume=5"}
	plain := library.ComicBook{ID: "p1", Series: "Plain", FilePath: "c.cbz"}
	fileless := library.ComicBook{ID: "f1", Series: "Wanted", Tags: "CVDBSKIP"}
	s := newDuplicatesTestServer(t, t.TempDir(), []library.ComicBook{stuck, tagged, plain, fileless})

	if n := s.MigrateCVDBSkip(); n != 1 {
		t.Fatalf("first run changed %d, want 1", n)
	}
	if n := s.MigrateCVDBSkip(); n != 0 {
		t.Errorf("second run changed %d, want 0 (idempotent)", n)
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/s1", &d)
	if d.NoCVMatch == nil || d.NoCVMatch.Reason != workflow.NoCVMatchLegacy {
		t.Fatalf("marker = %+v", d.NoCVMatch)
	}
	if d.Stage == "scrape" || d.Stage == "" {
		t.Errorf("stage = %q, want past scrape", d.Stage)
	}
	if !strings.Contains(d.Book.Tags, "CVDBSKIP") {
		t.Errorf("tag was removed: %q", d.Book.Tags)
	}
	for _, id := range []string{"t1", "p1", "f1"} {
		d = BookDetail{}
		getJSON(t, s, "/api/library/books/"+id, &d)
		if d.NoCVMatch != nil {
			t.Errorf("%s was marked", id)
		}
	}
}

func TestCVNoMatchBulk_MarksListsAndUndoes(t *testing.T) {
	var books []library.ComicBook
	for _, id := range []string{"b1", "b2", "b3"} {
		b := library.ComicBook{ID: id, Series: "Omnibus " + id, FilePath: id + ".cbz", ScanInformation: "s"}
		workflow.SetStage(&b, workflow.StageScrape)
		books = append(books, b)
	}
	books = append(books, library.ComicBook{ID: "w1", Series: "Wanted"})
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	bulk := func(body string) (int, CVNoMatchBulkResult) {
		w := httptest.NewRecorder()
		s.handleCVNoMatchBooks(w, httptest.NewRequest(http.MethodPost, "/api/library/workflow/cv-nomatch", strings.NewReader(body)))
		var res CVNoMatchBulkResult
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, res
	}
	type page struct {
		Comics []CVNoMatchPreview `json:"comics"`
		Total  int                `json:"total"`
	}
	list := func() page {
		var p page
		w := httptest.NewRecorder()
		s.handleCVNoMatchBooks(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow/cv-nomatch?limit=10", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("list status %d", w.Code)
		}
		if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	for _, bad := range []string{`{}`, `{"book_ids":["b1"],"set":true,"reason":"bogus"}`, `not json`} {
		if c, _ := bulk(bad); c != http.StatusBadRequest {
			t.Errorf("%s -> %d, want 400", bad, c)
		}
	}

	c, res := bulk(`{"book_ids":["b1","b2","b2","w1","nope"],"set":true,"reason":"collected"}`)
	if c != http.StatusOK || res.Changed != 2 || res.Unchanged != 0 || len(res.Skipped) != 2 {
		t.Fatalf("mark = %d %+v", c, res)
	}
	p := list()
	if p.Total != 2 || len(p.Comics) != 2 || p.Comics[0].Reason != "collected" || p.Comics[0].Date == "" {
		t.Fatalf("list after mark = %+v", p)
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/b2", &d)
	if d.Stage == "scrape" {
		t.Errorf("b2 still at scrape after a bulk mark")
	}

	// Marking again changes nothing; undo puts the book back at Scrape.
	if c, res = bulk(`{"book_ids":["b1"],"set":true,"reason":"other"}`); c != http.StatusOK || res.Changed+res.Unchanged != 1 {
		t.Fatalf("re-mark = %d %+v", c, res)
	}
	if c, res = bulk(`{"book_ids":["b1","b3"],"set":false}`); c != http.StatusOK || res.Changed != 1 || res.Unchanged != 1 {
		t.Fatalf("undo = %d %+v", c, res)
	}
	if p = list(); p.Total != 1 || p.Comics[0].ID != "b2" {
		t.Fatalf("list after undo = %+v", p)
	}
	d = BookDetail{}
	getJSON(t, s, "/api/library/books/b1", &d)
	if d.Stage != "scrape" || d.NoCVMatch != nil {
		t.Errorf("b1 after undo: stage %q mark %+v", d.Stage, d.NoCVMatch)
	}
}

func TestBookCollects_SetShowClearAuditedAndKeepsKindApart(t *testing.T) {
	books := []library.ComicBook{{ID: "b1", Series: "Lazarus", Number: "3", FilePath: "a.cbz"}}
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	post := func(body string) (int, string) {
		w := httptest.NewRecorder()
		s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/b1/collects", strings.NewReader(body)))
		return w.Code, w.Body.String()
	}
	if code, _ := post(`{"entries":[{"from":"1"}]}`); code != http.StatusBadRequest {
		t.Errorf("entry without volume or series: status %d", code)
	}
	body := `{"entries":[{"volume":4050,"series":"Ghost Rider","from":"1","to":"5"},{"series":"Heaven's On Fire","from":"1","to":"6"}]}`
	if code, out := post(body); code != http.StatusOK {
		t.Fatalf("set: status %d %s", code, out)
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/b1", &d)
	if len(d.Collects) != 2 || d.Collects[0].Volume != 4050 || d.Collects[1].To != "6" {
		t.Errorf("collects = %+v", d.Collects)
	}
	rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{BookID: "b1", Limit: 10})
	if len(rows) != 1 || rows[0].Action != "collects" {
		t.Errorf("audit rows = %+v", rows)
	}
	if code, _ := post(`{"entries":[]}`); code != http.StatusOK {
		t.Fatalf("clear: status %d", code)
	}
	getJSON(t, s, "/api/library/books/b1", &d)
	if len(d.Collects) != 0 {
		t.Errorf("after clear: %+v", d.Collects)
	}
}

func TestBookCollects_PreviewSavesNothingAndLookupNeedsScraper(t *testing.T) {
	books := []library.ComicBook{{ID: "b1", Series: "Lazarus", Number: "3", FilePath: "a.cbz"}}
	s := newDuplicatesTestServer(t, t.TempDir(), books)
	w := httptest.NewRecorder()
	body := `{"preview":true,"entries":[{"series":"Ghost Rider","from":"5","to":"1"}]}`
	s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/b1/collects", strings.NewReader(body)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "backwards") {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var d BookDetail
	getJSON(t, s, "/api/library/books/b1", &d)
	if len(d.Collects) != 0 {
		t.Errorf("preview saved links: %+v", d.Collects)
	}
	w = httptest.NewRecorder()
	s.handleBooksRouter(w, httptest.NewRequest(http.MethodPost, "/api/library/books/b1/collects/lookup", strings.NewReader(`{"ref":"4050-1"}`)))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("lookup without scraper: %d", w.Code)
	}
}
