package workflow

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestGetSetStage_RoundTrip(t *testing.T) {
	book := &library.ComicBook{}
	if got := GetStage(book); got != StageUnknown {
		t.Fatalf("GetStage on a fresh book = %v, want StageUnknown", got)
	}

	SetStage(book, StageScanInfo)
	if got := GetStage(book); got != StageScanInfo {
		t.Errorf("GetStage after SetStage(ScanInfo) = %v, want StageScanInfo", got)
	}

	// Overwriting must replace, not duplicate, the stored value.
	SetStage(book, StageDataManager)
	if got := GetStage(book); got != StageDataManager {
		t.Errorf("GetStage after overwrite = %v, want StageDataManager", got)
	}
}

// TestGetStage_FilelessBookNeverReportsToMove is the regression test for a
// real user report (comic-server-of7, screenshot): a book with no
// FilePath (e.g. a ComicRack "wanted" placeholder for an issue not yet
// owned) showed up in the To Move drill-in with a blank Current Path -
// there's nothing to move. GetStage must down-report StageToMove to
// StageDataManager for such a book even if that's what's literally stored,
// so a book mis-staged before this invariant existed self-heals without a
// separate backfill.
func TestGetStage_FilelessBookNeverReportsToMove(t *testing.T) {
	book := &library.ComicBook{FilePath: ""}
	SetStage(book, StageToMove)
	if got := GetStage(book); got != StageDataManager {
		t.Errorf("GetStage on a fileless book explicitly staged ToMove = %v, want StageDataManager", got)
	}

	// A book WITH a file must still report StageToMove normally - this
	// invariant is scoped to fileless books only.
	withFile := &library.ComicBook{FilePath: "/comics/book.cbz"}
	SetStage(withFile, StageToMove)
	if got := GetStage(withFile); got != StageToMove {
		t.Errorf("GetStage on a book with a file = %v, want StageToMove", got)
	}
}

// aceRuleset mirrors a real dataman.dat shape: a rule matching on
// Publisher, an action setting SeriesGroup - used to exercise
// InferStage's idempotent Data Manager check with something concrete.
var aceRuleset = []datamanager.Ruleset{
	{
		Name: "Ace Family",
		Mode: "AND",
		Rules: []datamanager.Rule{
			{Field: "Publisher", Modifier: "Is", Value: "Ace Magazines"},
		},
		Actions: []datamanager.Action{
			{Field: "SeriesGroup", Modifier: "SetValue", Value: "Ace Family"},
		},
	},
}

func TestInferStage_RealWorldConditions(t *testing.T) {
	tests := []struct {
		name     string
		book     *library.ComicBook
		rulesets []datamanager.Ruleset
		want     Stage
	}{
		{
			name: "not a cbz - needs conversion",
			book: &library.ComicBook{FilePath: "Batman 001.cbr"},
			want: StageConvertToCBZ,
		},
		{
			name: "already cbz but no comicvine tag - needs scrape",
			book: &library.ComicBook{FilePath: "Batman 001.cbz"},
			want: StageScrape,
		},
		{
			name: "tagged but no scan info - needs scan info",
			book: &library.ComicBook{
				FilePath:          "Batman 001.cbz",
				CustomValuesStore: ",comicvine_volume=1234",
			},
			want: StageScanInfo,
		},
		{
			name: "scanned, Data Manager rule still has a pending change - needs data manager",
			book: &library.ComicBook{
				FilePath:          "Batman 001.cbz",
				CustomValuesStore: ",comicvine_volume=1234",
				ScanInformation:   "Scanner:Zeta-Fictscans",
				Publisher:         "Ace Magazines",
			},
			rulesets: aceRuleset,
			want:     StageDataManager,
		},
		{
			name: "scanned, Data Manager rule already satisfied (idempotent - no changes) - ready to move",
			book: &library.ComicBook{
				FilePath:          "Batman 001.cbz",
				CustomValuesStore: ",comicvine_volume=1234",
				ScanInformation:   "Scanner:Zeta-Fictscans",
				Publisher:         "Ace Magazines",
				SeriesGroup:       "Ace Family", // rule's action would produce this exact value
			},
			rulesets: aceRuleset,
			want:     StageToMove,
		},
		{
			name: "no rules configured at all - nothing pending, ready to move",
			book: &library.ComicBook{
				FilePath:          "Batman 001.cbz",
				CustomValuesStore: ",comicvine_volume=1234",
				ScanInformation:   "Scanner:Zeta-Fictscans",
			},
			rulesets: nil,
			want:     StageToMove,
		},
		{
			name: "fileless placeholder book - nothing to move, caps at data manager",
			book: &library.ComicBook{
				FilePath:          "",
				CustomValuesStore: ",comicvine_volume=1234",
				ScanInformation:   "Scanner:Zeta-Fictscans",
			},
			rulesets: nil,
			want:     StageDataManager,
		},
		{
			name: ".zip counts as already-converted, same as .cbz",
			book: &library.ComicBook{
				FilePath:          "Batman 001.zip",
				CustomValuesStore: ",comicvine_volume=1234",
				ScanInformation:   "Scanner:Zeta-Fictscans",
			},
			rulesets: nil,
			want:     StageToMove,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferStage(tt.book, tt.rulesets); got != tt.want {
				t.Errorf("InferStage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInferStage_DataManagerCheckNeverMutatesTheRealBook(t *testing.T) {
	book := &library.ComicBook{
		FilePath:          "Batman 001.cbz",
		CustomValuesStore: ",comicvine_volume=1234",
		ScanInformation:   "Scanner:Zeta-Fictscans",
		Publisher:         "Ace Magazines",
	}
	if got := InferStage(book, aceRuleset); got != StageDataManager {
		t.Fatalf("InferStage() = %v, want StageDataManager", got)
	}
	if book.SeriesGroup != "" {
		t.Errorf("InferStage must not mutate the real book, but SeriesGroup = %q", book.SeriesGroup)
	}
}

func TestAdvanceIfAtOrBefore_MovesForwardOnly(t *testing.T) {
	book := &library.ComicBook{FilePath: "Batman 001.cbz"} // infers to StageScrape

	// Completing Scrape should advance an inferred-Scrape book to ScanInfo.
	if !AdvanceIfAtOrBefore(book, StageScrape, nil) {
		t.Fatal("expected AdvanceIfAtOrBefore(Scrape) to report a change")
	}
	if got := GetStage(book); got != StageScanInfo {
		t.Fatalf("GetStage after advancing past Scrape = %v, want StageScanInfo", got)
	}

	// Re-running the SAME (now past) stage's completion must not regress
	// the book backward.
	if AdvanceIfAtOrBefore(book, StageScrape, nil) {
		t.Error("expected no change re-running Scrape's completion on a book already past it")
	}
	if got := GetStage(book); got != StageScanInfo {
		t.Errorf("stage regressed after re-running an already-passed stage: got %v, want StageScanInfo", got)
	}

	// Completing a LATER stage than current (e.g. someone manually runs
	// Data Manager on a book that was never scraped) still advances -
	// the field tracks "furthest known-completed stage", not a strict
	// checklist that must be satisfied in order. It never REGRESSES
	// (covered above) but an out-of-order completion is taken at face
	// value, not rejected.
	fresh := &library.ComicBook{FilePath: "Batman 002.cbr"} // infers to StageConvertToCBZ
	if !AdvanceIfAtOrBefore(fresh, StageDataManager, nil) {
		t.Error("expected an out-of-order completion to still advance the book")
	}
	if got := GetStage(fresh); got != StageToMove {
		t.Errorf("GetStage after out-of-order DataManager completion = %v, want StageToMove", got)
	}
}

func TestAdvanceIfAtOrBefore_ToMoveAdvancesToOrganized(t *testing.T) {
	book := &library.ComicBook{}
	SetStage(book, StageToMove)
	if !AdvanceIfAtOrBefore(book, StageToMove, nil) {
		t.Error("expected ToMove to advance now that comic-server-3bz.5 exists to complete it")
	}
	if got := GetStage(book); got != StageOrganized {
		t.Errorf("GetStage = %v, want StageOrganized", got)
	}
}

func TestAdvanceIfAtOrBefore_TerminalStageStaysPut(t *testing.T) {
	book := &library.ComicBook{}
	SetStage(book, StageOrganized)
	if AdvanceIfAtOrBefore(book, StageOrganized, nil) {
		t.Error("expected no change - StageOrganized is the true terminal stage")
	}
	if got := GetStage(book); got != StageOrganized {
		t.Errorf("GetStage = %v, want StageOrganized unchanged", got)
	}
}

func TestRegressToStageIfPast_OrganizedRegressesToToMove(t *testing.T) {
	book := &library.ComicBook{FilePath: "/comics/book.cbz"}
	SetStage(book, StageOrganized)
	if !RegressToStageIfPast(book, StageToMove) {
		t.Error("expected a change - StageOrganized is past StageToMove")
	}
	if got := GetStage(book); got != StageToMove {
		t.Errorf("GetStage = %v, want StageToMove", got)
	}
}

func TestRegressToStageIfPast_AlreadyAtTargetIsNoOp(t *testing.T) {
	book := &library.ComicBook{FilePath: "/comics/book.cbz"}
	SetStage(book, StageToMove)
	if RegressToStageIfPast(book, StageToMove) {
		t.Error("expected no change - already at target, not past it")
	}
	if got := GetStage(book); got != StageToMove {
		t.Errorf("GetStage = %v, want StageToMove unchanged", got)
	}
}

func TestRegressToStageIfPast_BeforeTargetIsUntouched(t *testing.T) {
	book := &library.ComicBook{}
	SetStage(book, StageScanInfo)
	if RegressToStageIfPast(book, StageToMove) {
		t.Error("expected no change - StageScanInfo is before StageToMove, never advanced by this")
	}
	if got := GetStage(book); got != StageScanInfo {
		t.Errorf("GetStage = %v, want StageScanInfo unchanged", got)
	}
}

func TestRegressToStageIfPast_NeverStagedBookIsUntouched(t *testing.T) {
	book := &library.ComicBook{}
	if RegressToStageIfPast(book, StageToMove) {
		t.Error("expected no change - a never-staged book must not be pulled into the pipeline by this")
	}
	if got := GetStage(book); got != StageUnknown {
		t.Errorf("GetStage = %v, want StageUnknown unchanged", got)
	}
}

func TestIsCVDBSkip(t *testing.T) {
	tests := []struct {
		tags string
		want bool
	}{
		{"", false},
		{"CVDBSKIP", true},
		{"Dawn of DC, CVDBSKIP", true},
		{"Dawn of DC, CVDBSKIPPED", false}, // must not substring-match
	}
	for _, tt := range tests {
		book := &library.ComicBook{Tags: tt.tags}
		if got := IsCVDBSkip(book); got != tt.want {
			t.Errorf("IsCVDBSkip(Tags=%q) = %v, want %v", tt.tags, got, tt.want)
		}
	}
}

func TestStageIndex(t *testing.T) {
	if StageIndex(StageUnknown) != 0 {
		t.Errorf("StageIndex(StageUnknown) = %d, want 0", StageIndex(StageUnknown))
	}
	if StageIndex(StageConvertToCBZ) != 1 {
		t.Errorf("StageIndex(StageConvertToCBZ) = %d, want 1", StageIndex(StageConvertToCBZ))
	}
	if StageIndex(StageOrganized) != len(Stages) {
		t.Errorf("StageIndex(StageOrganized) = %d, want %d", StageIndex(StageOrganized), len(Stages))
	}
}
