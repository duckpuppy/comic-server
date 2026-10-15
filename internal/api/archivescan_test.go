package api

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

func writeGoodCBZ(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("001.jpg")
	_, _ = w.Write([]byte("x"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestArchiveScan_FlagsUnreadCorruptFileAndResumes(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.cbz")
	bad := filepath.Join(dir, "bad.cbz")
	writeGoodCBZ(t, good)
	if err := os.WriteFile(bad, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := healthServer(t, []library.ComicBook{
		{ID: "good", Series: "A", Number: "1", FilePath: good},
		{ID: "bad", Series: "B", Number: "1", FilePath: bad},
		{ID: "gone", Series: "C", Number: "1", FilePath: filepath.Join(dir, "gone.cbz")},
		{ID: "pdf", Series: "D", Number: "1", FilePath: filepath.Join(dir, "x.pdf")},
		{ID: "wanted", Series: "E", Number: "1"},
	})
	comicvine.SetArchiveFailureHook(s.RecordArchiveFailure)
	t.Cleanup(func() { comicvine.SetArchiveFailureHook(nil) })

	res := s.scanArchives(context.Background(), 0)
	if res.Opened != 2 || res.Corrupt != 1 || res.Missing != 1 || res.Failed != 0 {
		t.Fatalf("first pass = %+v", res)
	}
	if got := flaggedIDs(t, s, attention.FlagCorruptedArchive); len(got) != 1 || got[0] != "bad" {
		t.Fatalf("corrupted flag = %v, want [bad]", got)
	}

	// Second pass reads nothing: the good file is remembered, the bad one is
	// already recorded at this version.
	res = s.scanArchives(context.Background(), 0)
	if res.Opened != 0 || res.Skipped != 2 {
		t.Fatalf("second pass = %+v", res)
	}

	// A changed file is read again.
	writeGoodCBZ(t, bad)
	res = s.scanArchives(context.Background(), 0)
	if res.Opened != 1 || res.Corrupt != 0 {
		t.Fatalf("after fixing the file = %+v", res)
	}
}

func TestArchiveScan_StopsWhenCanceled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.cbz")
	writeGoodCBZ(t, p)
	s := healthServer(t, []library.ComicBook{{ID: "a", Series: "A", Number: "1", FilePath: p}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := s.scanArchives(ctx, 0)
	if !res.Canceled || res.Opened != 0 {
		t.Errorf("canceled pass = %+v", res)
	}
}
