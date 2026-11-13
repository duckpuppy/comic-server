package watchfolder

import (
	"archive/zip"
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

func writeZipWith(t *testing.T, path string, names ...string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("x"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// A .zip/.7z/.rar is listed only when it holds image pages. The "cb*" names
// are always listed, as before.
func TestScan_AmbiguousArchivesNeedImagePages(t *testing.T) {
	dir := t.TempDir()
	comicZip := filepath.Join(dir, "Real Comic 001.zip")
	dataZip := filepath.Join(dir, "backup.zip")
	broken7z := filepath.Join(dir, "notes.7z")
	cbz := filepath.Join(dir, "Plain 001.cbz")
	writeZipWith(t, comicZip, "page001.jpg", "page002.jpg")
	writeZipWith(t, dataZip, "readme.txt", "data.csv")
	for _, p := range []string{broken7z, cbz} {
		if err := os.WriteFile(p, []byte("not an archive"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	found, err := Scan([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range found {
		got = append(got, filepath.Base(f.Path))
	}
	want := []string{"Plain 001.cbz", "Real Comic 001.zip"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("found %v, want %v", got, want)
	}
}

// Opening the same unchanged archive again is skipped; a changed file is checked again.
func TestScan_ImageCheckIsRememberedUntilTheFileChanges(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "maybe.zip")
	if err := os.WriteFile(p, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	orig := listPages
	listPages = func(string) ([]string, error) { calls++; return []string{"a.jpg"}, nil }
	defer func() { listPages = orig }()

	for i := 0; i < 3; i++ {
		if _, err := Scan([]string{dir}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("archive opened %d times for 3 scans, want 1", calls)
	}
	if err := os.WriteFile(p, []byte("a longer body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan([]string{dir}, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("changed archive was not checked again (calls=%d)", calls)
	}
}
