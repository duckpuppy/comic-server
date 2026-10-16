package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestScrapeLog_RecordsJobStepsForTheDialog(t *testing.T) {
	books := []library.ComicBook{{ID: "book-1", FilePath: "Batman 001.cbz"}}
	fx := &scrapeAPIFixture{
		searchResults: map[string][]comicvine.Volume{
			"Batman": {{ID: 100, Name: "Batman", StartYear: "2016", CountOfIssues: 85}},
		},
		issuesByVolume: map[int][]comicvine.Issue{100: {{ID: 1001, IssueNumber: "1"}}},
		issueDetails:   map[int]*comicvine.IssueDetail{1001: {ID: 1001, IssueNumber: "1", Name: "I Am Gotham"}},
	}
	srv := newScrapeAPITestServer(t, books, fx)

	w := httptest.NewRecorder()
	srv.handleScrapeStart(w, httptest.NewRequest(http.MethodPost, "/api/scrape", strings.NewReader(`{"book_ids":["book-1"]}`)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var started struct {
		JobID string `json:"job_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &started)
	waitForJobStatus(t, srv, comicvine.JobStatusComplete)

	w = httptest.NewRecorder()
	srv.handleScrapeLog(w, httptest.NewRequest(http.MethodGet, "/api/scrape/log?job_id="+started.JobID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("log status = %d", w.Code)
	}
	var got struct {
		Running bool            `json:"running"`
		Lines   []scrapeLogLine `json:"lines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Running {
		t.Error("finished job reported as running")
	}
	var text []string
	for i, l := range got.Lines {
		if l.Seq != i+1 {
			t.Errorf("line %d has seq %d", i, l.Seq)
		}
		text = append(text, l.Msg)
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{`Searching ComicVine for "Batman"`, "Book: Batman 001.cbz", "Best match: Batman (2016)", "Looking for issue #1", "Matched: volume 100, issue 1001", "Finished: 1 matched"} {
		if !strings.Contains(all, want) {
			t.Errorf("log lacks %q:\n%s", want, all)
		}
	}

	w = httptest.NewRecorder()
	srv.handleScrapeLog(w, httptest.NewRequest(http.MethodGet, "/api/scrape/log", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing job_id: status = %d, want 400", w.Code)
	}
}

func TestScrapeLogStore_CapsLinesAndJobs(t *testing.T) {
	st := newScrapeLogStore()
	for range scrapeLogMaxLines + 5 {
		st.add("a", comicvine.JobLogInfo, "x")
	}
	lines := st.lines("a")
	if len(lines) != scrapeLogMaxLines || lines[0].Seq != 6 {
		t.Errorf("kept %d lines from seq %d; want %d from 6", len(lines), lines[0].Seq, scrapeLogMaxLines)
	}
	for i := range scrapeLogMaxJobs {
		st.add(string(rune('b'+i)), comicvine.JobLogInfo, "x")
	}
	if len(st.lines("a")) != 0 {
		t.Error("oldest job's log should be dropped")
	}
	var nilStore *scrapeLogStore
	if l := nilStore.lines("a"); l == nil || len(l) != 0 {
		t.Error("nil store should return an empty list")
	}
}
