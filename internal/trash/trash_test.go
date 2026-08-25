package trash

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	if _, err := New("", 30); err == nil {
		t.Error("expected error for empty root")
	}
	if _, err := New("/tmp/x", 0); err == nil {
		t.Error("expected error for zero retentionDays")
	}
	if _, err := New("/tmp/x", -1); err == nil {
		t.Error("expected error for negative retentionDays")
	}
	tr, err := New("/tmp/x", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Root != "/tmp/x" || tr.RetentionDays != 30 {
		t.Errorf("unexpected Trash: %+v", tr)
	}
}

func TestReplace_Success(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	target := filepath.Join(libDir, "book.cbz")

	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := &Trash{Root: trashDir, RetentionDays: 30}
	err := tr.Replace(target,
		func(tmpPath string) error {
			return os.WriteFile(tmpPath, []byte("new content"), 0o644)
		},
		func(tmpPath string) error {
			data, err := os.ReadFile(tmpPath)
			if err != nil {
				return err
			}
			if string(data) != "new content" {
				return errors.New("unexpected content")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target missing after replace: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("target has wrong content: %q", data)
	}

	// The original should be quarantined somewhere under trashDir, not deleted.
	found := findFileContaining(t, trashDir, "old content")
	if found == "" {
		t.Fatal("original content not found anywhere under trash root")
	}
	if !strings.Contains(filepath.Base(found), "book.cbz"+trashSuffixSep) {
		t.Errorf("quarantined file has unexpected name: %s", found)
	}

	// No leftover temp files in the library dir.
	entries, _ := os.ReadDir(libDir)
	if len(entries) != 1 || entries[0].Name() != "book.cbz" {
		t.Errorf("unexpected leftover files in library dir: %v", entries)
	}
}

func TestReplace_ValidateFailureLeavesOriginalUntouched(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	target := filepath.Join(libDir, "book.cbz")

	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := &Trash{Root: trashDir, RetentionDays: 30}
	err := tr.Replace(target,
		func(tmpPath string) error {
			return os.WriteFile(tmpPath, []byte("bad new content"), 0o644)
		},
		func(tmpPath string) error {
			return errors.New("simulated validation failure")
		},
	)
	if err == nil {
		t.Fatal("expected error from failed validation")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("original file missing after failed validate: %v", err)
	}
	if string(data) != "old content" {
		t.Errorf("original file was modified: %q", data)
	}

	if found := findFileContaining(t, trashDir, "bad new content"); found != "" {
		t.Errorf("failed replace should not have quarantined anything, found: %s", found)
	}

	entries, _ := os.ReadDir(libDir)
	if len(entries) != 1 {
		t.Errorf("temp file was not cleaned up: %v", entries)
	}
}

func TestReplace_WriteFailureLeavesOriginalUntouched(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	target := filepath.Join(libDir, "book.cbz")

	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := &Trash{Root: trashDir, RetentionDays: 30}
	err := tr.Replace(target,
		func(tmpPath string) error {
			return errors.New("simulated write failure")
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected error from failed write")
	}

	data, err := os.ReadFile(target)
	if err != nil || string(data) != "old content" {
		t.Fatalf("original file was disturbed: data=%q err=%v", data, err)
	}
}

func TestReplace_CrossFilesystemQuarantineFallsBackToCopy(t *testing.T) {
	// Same as TestReplace_Success but exercises moveFile's copy+remove
	// fallback path directly, since t.TempDir() dirs are typically on the
	// same filesystem and won't naturally trigger EXDEV in CI.
	libDir := t.TempDir()
	trashDir := t.TempDir()
	target := filepath.Join(libDir, "book.cbz")
	if err := os.WriteFile(target, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(trashDir, "sub", "book.cbz~123")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyThenRemove(target, dst); err != nil {
		t.Fatalf("copyThenRemove failed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("source should be removed after copyThenRemove, stat err: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "old content" {
		t.Errorf("dst has wrong content: data=%q err=%v", data, err)
	}
}

func TestQuarantinePathFor_NoCollisionOnRepeatedReplace(t *testing.T) {
	libDir := t.TempDir()
	trashDir := t.TempDir()
	target := filepath.Join(libDir, "book.cbz")
	tr := &Trash{Root: trashDir, RetentionDays: 30}

	p1 := tr.quarantinePathFor(target, time.Unix(1000, 0))
	p2 := tr.quarantinePathFor(target, time.Unix(2000, 0))
	if p1 == p2 {
		t.Errorf("expected distinct quarantine paths for different timestamps, got %s twice", p1)
	}
}

func TestSweep_RemovesOnlyOldEntries(t *testing.T) {
	trashDir := t.TempDir()
	tr := &Trash{Root: trashDir, RetentionDays: 30}
	now := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)

	old := filepath.Join(trashDir, "lib", "old.cbz~"+strconv.FormatInt(now.Add(-31*24*time.Hour).Unix(), 10))
	recent := filepath.Join(trashDir, "lib", "recent.cbz~"+strconv.FormatInt(now.Add(-1*24*time.Hour).Unix(), 10))
	boundary := filepath.Join(trashDir, "lib", "boundary.cbz~"+strconv.FormatInt(now.Add(-30*24*time.Hour).Unix(), 10))
	unrelated := filepath.Join(trashDir, "lib", "not-ours.txt")

	mustWrite(t, old, "x")
	mustWrite(t, recent, "x")
	mustWrite(t, boundary, "x")
	mustWrite(t, unrelated, "x")

	result := tr.Sweep(now)

	// boundary is exactly RetentionDays old - swept too (Sweep removes
	// anything at or past the cutoff, not strictly older than it).
	if result.Removed != 2 {
		t.Errorf("expected 2 removed, got %d (errs: %v)", result.Removed, result.Errs)
	}
	assertMissing(t, old)
	assertMissing(t, boundary)
	assertExists(t, recent)
	assertExists(t, unrelated)
}

func TestSweep_MissingRootIsNotAnError(t *testing.T) {
	tr := &Trash{Root: filepath.Join(t.TempDir(), "does-not-exist"), RetentionDays: 30}
	result := tr.Sweep(time.Now())
	if len(result.Errs) != 0 {
		t.Errorf("expected no errors for a missing trash root, got: %v", result.Errs)
	}
	if result.Removed != 0 {
		t.Errorf("expected 0 removed, got %d", result.Removed)
	}
}

func TestSweep_PrunesEmptyDirectories(t *testing.T) {
	trashDir := t.TempDir()
	tr := &Trash{Root: trashDir, RetentionDays: 30}
	now := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)

	old := filepath.Join(trashDir, "a", "b", "c", "old.cbz~"+strconv.FormatInt(now.Add(-60*24*time.Hour).Unix(), 10))
	mustWrite(t, old, "x")

	tr.Sweep(now)

	if _, err := os.Stat(filepath.Join(trashDir, "a")); !os.IsNotExist(err) {
		t.Errorf("expected empty directory tree to be pruned, stat err: %v", err)
	}
}

func TestParseTrashedAt(t *testing.T) {
	cases := []struct {
		name    string
		wantOK  bool
		wantSec int64
	}{
		{"book.cbz~1735689600", true, 1735689600},
		{"book.cbz", false, 0},
		{"book.cbz~notanumber", false, 0},
		{"book.cbz~", false, 0},
	}
	for _, c := range cases {
		got, ok := parseTrashedAt(c.name)
		if ok != c.wantOK {
			t.Errorf("parseTrashedAt(%q) ok = %v, want %v", c.name, ok, c.wantOK)
			continue
		}
		if ok && got.Unix() != c.wantSec {
			t.Errorf("parseTrashedAt(%q) = %d, want %d", c.name, got.Unix(), c.wantSec)
		}
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	tr := &Trash{Root: t.TempDir(), RetentionDays: 30}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	calls := 0
	go func() {
		tr.Run(ctx, 10*time.Millisecond, func(SweepResult) { calls++ })
		close(done)
	}()

	time.Sleep(35 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
	if calls < 1 {
		t.Error("expected at least the initial sweep to have run")
	}
}

// --- test helpers ---

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to be gone, stat err: %v", path, err)
	}
}

func findFileContaining(t *testing.T, root, content string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr == nil && string(data) == content {
			found = path
		}
		return nil
	})
	return found
}
