package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/storage"
)

const sampleLibraryForImportTest = `<?xml version="1.0" encoding="utf-8"?>
<ComicDatabase xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" Id="test-library">
  <Books>
    <Book Id="book-1" File="G:\Comics\Atomic War! (1952)\Atomic War! #001 (1952).cbz">
      <Series>Atomic War!</Series>
      <Title>Atomic War! #1</Title>
      <Number>1</Number>
      <Year>1952</Year>
      <Publisher>Ace Magazines</Publisher>
    </Book>
  </Books>
  <ComicLists />
  <ReadingLists />
</ComicDatabase>`

// TestRunImport_TranslatesFilePathThroughConfiguredMount is the regression
// test for comic-server-q7f: a book's File path is recorded in ComicDb.xml
// exactly as the Windows host that ran ComicRack saw it ("G:\Comics\..."),
// which does not exist on the machine comic-server actually runs on (a
// Docker container, most commonly). library.db should store the real,
// translated path directly rather than the raw one - the same translation
// already applied at read time via config.Config.ResolveLibraryFilePath,
// now also applied once here at import time.
func TestRunImport_TranslatesFilePathThroughConfiguredMount(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`server:
  library_source_root: 'G:\Comics'
  library_root: /mnt/comics
`), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	xmlPath := filepath.Join(dir, "ComicDb.xml")
	if err := os.WriteFile(xmlPath, []byte(sampleLibraryForImportTest), 0o644); err != nil {
		t.Fatalf("write ComicDb.xml: %v", err)
	}

	dbPath := filepath.Join(dir, "library.db")

	origConfigFile, origXML, origDB, origDryRun, origVerbose := configFile, importXMLPath, importDBPath, importDryRun, importVerbose
	t.Cleanup(func() {
		configFile, importXMLPath, importDBPath, importDryRun, importVerbose = origConfigFile, origXML, origDB, origDryRun, origVerbose
	})
	configFile = configPath
	importXMLPath = xmlPath
	importDBPath = dbPath
	importDryRun = false
	importVerbose = false

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport: %v", err)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open library.db: %v", err)
	}
	defer db.Close()

	book, err := db.GetBook("book-1")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if book.FilePath != "/mnt/comics/Atomic War! (1952)/Atomic War! #001 (1952).cbz" {
		t.Errorf("FilePath = %q, want translated path under /mnt/comics", book.FilePath)
	}
}

// TestRunImport_NoMountConfiguredLeavesFilePathAsIs covers the common case
// (no library_source_root/library_root set) - FilePath must pass
// through unchanged.
func TestRunImport_NoMountConfiguredLeavesFilePathAsIs(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("server: {}\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	xmlPath := filepath.Join(dir, "ComicDb.xml")
	if err := os.WriteFile(xmlPath, []byte(sampleLibraryForImportTest), 0o644); err != nil {
		t.Fatalf("write ComicDb.xml: %v", err)
	}

	dbPath := filepath.Join(dir, "library.db")

	origConfigFile, origXML, origDB, origDryRun, origVerbose := configFile, importXMLPath, importDBPath, importDryRun, importVerbose
	t.Cleanup(func() {
		configFile, importXMLPath, importDBPath, importDryRun, importVerbose = origConfigFile, origXML, origDB, origDryRun, origVerbose
	})
	configFile = configPath
	importXMLPath = xmlPath
	importDBPath = dbPath
	importDryRun = false
	importVerbose = false

	if err := runImport(nil, nil); err != nil {
		t.Fatalf("runImport: %v", err)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open library.db: %v", err)
	}
	defer db.Close()

	book, err := db.GetBook("book-1")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	want := `G:\Comics\Atomic War! (1952)\Atomic War! #001 (1952).cbz`
	if book.FilePath != want {
		t.Errorf("FilePath = %q, want %q unchanged", book.FilePath, want)
	}
}
