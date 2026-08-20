package komga

import (
	"errors"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestStatusStore_RecordAndSnapshot(t *testing.T) {
	s := NewStatusStore()

	s.Record(TargetResult{
		Target:       Target{ListID: "{list-1}", KomgaName: "Batman Collection", Type: TargetCollection},
		MatchedCount: 5,
		Unmatched: []UnmatchedBook{
			{Book: &library.ComicBook{ID: "1", Series: "Batman", Number: "1", FilePath: "/data/x.cbz"}, Reason: "no Komga series at /data/x"},
		},
	})

	snap := s.Snapshot()
	if len(snap.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(snap.Targets))
	}
	ts := snap.Targets[0]
	if ts.ListID != "{list-1}" || ts.KomgaName != "Batman Collection" || ts.MatchedCount != 5 {
		t.Errorf("unexpected target status: %+v", ts)
	}
	if len(ts.Unmatched) != 1 || ts.Unmatched[0].BookID != "1" || ts.Unmatched[0].Series != "Batman" {
		t.Errorf("unexpected unmatched info: %+v", ts.Unmatched)
	}
	if ts.Error != "" {
		t.Errorf("expected no error, got %q", ts.Error)
	}
}

func TestStatusStore_RecordOverwritesPreviousResultForSameTarget(t *testing.T) {
	s := NewStatusStore()

	s.Record(TargetResult{Target: Target{ListID: "{list-1}", KomgaName: "X"}, MatchedCount: 1})
	s.Record(TargetResult{Target: Target{ListID: "{list-1}", KomgaName: "X"}, MatchedCount: 9})

	snap := s.Snapshot()
	if len(snap.Targets) != 1 {
		t.Fatalf("expected 1 target (overwritten, not duplicated), got %d", len(snap.Targets))
	}
	if snap.Targets[0].MatchedCount != 9 {
		t.Errorf("expected latest MatchedCount 9, got %d", snap.Targets[0].MatchedCount)
	}
}

func TestStatusStore_PreservesFirstSeenOrder(t *testing.T) {
	s := NewStatusStore()

	s.Record(TargetResult{Target: Target{ListID: "b", KomgaName: "B"}})
	s.Record(TargetResult{Target: Target{ListID: "a", KomgaName: "A"}})
	s.Record(TargetResult{Target: Target{ListID: "b", KomgaName: "B"}}) // re-record b, shouldn't move it

	snap := s.Snapshot()
	if len(snap.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(snap.Targets))
	}
	if snap.Targets[0].ListID != "b" || snap.Targets[1].ListID != "a" {
		t.Errorf("expected first-seen order [b, a], got [%s, %s]", snap.Targets[0].ListID, snap.Targets[1].ListID)
	}
}

func TestStatusStore_TargetError(t *testing.T) {
	s := NewStatusStore()
	s.Record(TargetResult{
		Target: Target{ListID: "{list-1}", KomgaName: "X"},
		Err:    errors.New("push target failed"),
	})

	snap := s.Snapshot()
	if snap.Targets[0].Error != "push target failed" {
		t.Errorf("expected target error to be recorded, got %q", snap.Targets[0].Error)
	}
}

func TestStatusStore_IndexBuildErrorRecordedSeparately(t *testing.T) {
	s := NewStatusStore()

	// Zero Target with an error, as syncOnce emits when BuildIndex fails.
	s.Record(TargetResult{Err: errors.New("build komga index: request failed")})

	snap := s.Snapshot()
	if len(snap.Targets) != 0 {
		t.Errorf("expected 0 per-target entries for an index-build failure, got %d", len(snap.Targets))
	}
	if snap.LastIndexError != "build komga index: request failed" {
		t.Errorf("expected LastIndexError to be recorded, got %q", snap.LastIndexError)
	}
	if snap.LastIndexErrorTime == nil {
		t.Error("expected LastIndexErrorTime to be set")
	}
}

func TestStatusStore_EmptySnapshot(t *testing.T) {
	s := NewStatusStore()
	snap := s.Snapshot()
	if len(snap.Targets) != 0 {
		t.Errorf("expected empty snapshot, got %+v", snap)
	}
	if snap.LastIndexError != "" {
		t.Errorf("expected no index error, got %q", snap.LastIndexError)
	}
}
