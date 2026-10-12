package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/library"
)

func TestDiskCheck_FlagsOnlyMissingFiles(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "here.cbz")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rar := filepath.Join(dir, "rar.cbz")
	if err := os.WriteFile(rar, []byte("Rar!\x1a\x07\x01\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	books := []library.ComicBook{
		{ID: "4", Series: "D", FilePath: rar},
		{ID: "1", Series: "A", FilePath: present},
		{ID: "2", Series: "B", FilePath: filepath.Join(dir, "gone.cbz")},
		{ID: "3", Series: "C"}, // wanted placeholder
	}
	s := newCBZConvertTestServer(t, config.CBZConvertConfig{}, t.TempDir(), books)

	// Disabled by default: no flag, no goroutine.
	e, err := s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(e.booksByFlag[attention.FlagFileMissing]); n != 0 {
		t.Fatalf("disabled pass flagged %d books", n)
	}

	s.runDiskCheck() // synchronous run; also invalidates the cache
	s.SetDiskCheckInterval(time.Hour)
	e, err = s.getOrBuildWorkflowCache()
	if err != nil {
		t.Fatal(err)
	}
	got := e.booksByFlag[attention.FlagFileMissing]
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("file_missing = %+v, want only book 2", got)
	}
	mis := e.booksByFlag[attention.FlagMisnamedArchive]
	if len(mis) != 1 || mis[0].ID != "4" {
		t.Fatalf("misnamed_archive = %+v, want only book 4", mis)
	}
}
