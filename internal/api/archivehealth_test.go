package api

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

func healthServer(t *testing.T, books []library.ComicBook) *Server {
	t.Helper()
	return &Server{
		backend:  library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: books}, "", nil),
		config:   &config.Config{},
		configDB: newTestConfigDB(t),
	}
}

func flaggedIDs(t *testing.T, s *Server, f attention.Flag) []string {
	t.Helper()
	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range entry.booksByFlag[f] {
		ids = append(ids, b.ID)
	}
	return ids
}

func TestCorruptedArchive_RecordedFlaggedThenClearedWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.cbz")
	if err := os.WriteFile(badPath, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := healthServer(t, []library.ComicBook{
		{ID: "bad", Series: "A", Number: "1", FilePath: badPath},
		{ID: "fine", Series: "B", Number: "1", FilePath: filepath.Join(dir, "other.cbz")},
	})

	// Cache built before anything failed: no flag yet.
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 0 {
		t.Fatalf("before any failure: %v", got)
	}

	// A reader hits the corrupt file; the cached lists are patched in place.
	s.RecordArchiveFailure(badPath, errors.New("open cbz: zip: not a valid zip file"))
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 1 || got[0] != "bad" {
		t.Fatalf("after failure: %v", got)
	}

	// Recorded once per version: a second failure is a no-op, not a second flag.
	s.RecordArchiveFailure(badPath, errors.New("again"))
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 1 {
		t.Fatalf("repeat failure duplicated the flag: %v", got)
	}

	// It survives a cache rebuild (read from config.db).
	s.InvalidateWorkflowCache()
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 1 {
		t.Fatalf("after rebuild: %v", got)
	}

	// Replacing the file changes its version: the flag clears by itself.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(badPath, []byte("a different, longer replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.InvalidateWorkflowCache()
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 0 {
		t.Fatalf("after file changed: %v", got)
	}
	rows, _ := s.configDB.ArchiveFailures()
	if len(rows) != 0 {
		t.Errorf("stale row kept: %+v", rows)
	}
}

func TestCorruptedArchive_AcceptedFlagStaysHidden(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.cbz")
	_ = os.WriteFile(p, []byte("x"), 0o644)
	book := library.ComicBook{ID: "bad", Series: "A", Number: "1", FilePath: p}
	attention.SetAccepted(&book, attention.FlagCorruptedArchive, true)
	s := healthServer(t, []library.ComicBook{book})
	s.RecordArchiveFailure(p, errors.New("boom"))
	s.InvalidateWorkflowCache()
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 0 {
		t.Errorf("accepted flag shown: %v", got)
	}
}
