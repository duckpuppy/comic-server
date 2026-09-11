package watchfolder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan_FindsUnknownComicFilesOnly(t *testing.T) {
	dir := t.TempDir()
	newFile := filepath.Join(dir, "New Comic #1.cbz")
	knownFile := filepath.Join(dir, "Known Comic #1.cbz")
	notComic := filepath.Join(dir, "readme.txt")

	for _, p := range []string{newFile, knownFile, notComic} {
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	known := KnownPathSet([]string{knownFile})
	found, err := Scan([]string{dir}, known)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 discovered file, got %d: %+v", len(found), found)
	}
	if found[0].Path != newFile {
		t.Errorf("Path = %q, want %q", found[0].Path, newFile)
	}
	if found[0].Size != 4 {
		t.Errorf("Size = %d, want 4", found[0].Size)
	}
}

func TestScan_MissingFolderIsSkippedNotFatal(t *testing.T) {
	found, err := Scan([]string{filepath.Join(t.TempDir(), "does-not-exist")}, nil)
	if err != nil {
		t.Fatalf("Scan should not fail on a missing folder: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("expected no results, got %d", len(found))
	}
}

func TestScan_RecursesIntoSubfolders(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	nested := filepath.Join(sub, "Nested Comic #1.cbz")
	if err := os.WriteFile(nested, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	found, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 1 || found[0].Path != nested {
		t.Errorf("expected to find nested file, got %+v", found)
	}
}

func TestIsComicFile(t *testing.T) {
	cases := map[string]bool{
		"Batman.cbz": true, "Batman.CBR": true, "Batman.cb7": true,
		"Batman.zip": true, "readme.txt": false, "Batman.pdf": false,
	}
	for name, want := range cases {
		if got := IsComicFile(name); got != want {
			t.Errorf("IsComicFile(%q) = %v, want %v", name, got, want)
		}
	}
}
