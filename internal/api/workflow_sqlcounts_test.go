package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// noStageCounter hides the SQL counting method, so the summary falls back to
// the full snapshot: the reference answer the SQL counts must match.
type noStageCounter struct{ library.Backend }

func stagedBook(id, file string, st workflow.Stage) library.ComicBook {
	b := library.ComicBook{ID: id, FilePath: file, Series: "S" + id}
	workflow.SetStage(&b, st)
	return b
}

func summaryOf(t *testing.T, s *Server) WorkflowSummary {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleGetWorkflowSummary(w, httptest.NewRequest(http.MethodGet, "/api/library/workflow", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got WorkflowSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestWorkflowSummary_SQLCountsMatchTheSnapshot(t *testing.T) {
	skipped := stagedBook("skip", "skip.cbz", workflow.StageScrape)
	skipped.Tags = "x, CVDBSKIP"
	notInCV := stagedBook("nocv", "nocv.cbz", workflow.StageScrape)
	notInCV.CustomValuesStore = library.SetCustomValue(notInCV.CustomValuesStore, workflow.KeyNoCVMatch, "collected|2026-01-01")
	junk := library.ComicBook{ID: "junk", FilePath: "junk.cbz", Series: "J"}
	junk.CustomValuesStore = library.SetCustomValue(junk.CustomValuesStore, workflow.StageValueKey, "not_a_stage")
	books := []library.ComicBook{
		stagedBook("1", "1.cbz", workflow.StageConvertToCBZ),
		stagedBook("2", "2.cbz", workflow.StageScrape),
		stagedBook("3", "3.cbz", workflow.StageScrape),
		stagedBook("4", "4.cbz", workflow.StageScanInfo),
		stagedBook("5", "5.cbz", workflow.StageToMove),
		stagedBook("6", "6.cbz", workflow.StageOrganized),
		stagedBook("wanted", "", workflow.StageScrape),    // no file: in no stage
		{ID: "never", FilePath: "never.cbz", Series: "N"}, // has a file, never staged
		stagedBook("gone", "gone.cbz", workflow.StageToMove),
		skipped, notInCV, junk,
	}
	sb, err := storage.NewSQLiteBackend(filepath.Join(t.TempDir(), "lib.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sb.Close() })
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: books}, storage.ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := sb.DeleteBook("gone"); err != nil {
		t.Fatal(err)
	}

	for _, scanInfoOn := range []bool{true, false} {
		workflow.SetScanInfoEnabled(scanInfoOn)
		fast := summaryOf(t, &Server{backend: sb, config: &config.Config{}})
		slow := summaryOf(t, &Server{backend: noStageCounter{sb}, config: &config.Config{}})
		if !reflect.DeepEqual(fast, slow) {
			t.Errorf("scan info on=%v: SQL counts %+v differ from snapshot counts %+v", scanInfoOn, fast, slow)
		}
	}
	workflow.SetScanInfoEnabled(true)

	got := summaryOf(t, &Server{backend: sb, config: &config.Config{}})
	if got.Unstaged != 2 || got.CVDBSkip != 1 || got.CVNoMatch != 1 {
		t.Errorf("unstaged/cvdb/nocv = %d/%d/%d, want 2/1/1", got.Unstaged, got.CVDBSkip, got.CVNoMatch)
	}
}
