package sync

import (
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// TestComputeSyncPlan_CBLImportedReadingList is an end-to-end check for
// comic-server-hmld: does a CBL-imported reading list actually drive
// device sync planning, and does a reimport's membership change actually
// propagate to the next plan? TestSetFilterList/TestComputeSyncPlanWithFilter
// already cover the generic mechanism (any non-folder list type, smart
// lists specifically) - this one goes through the real
// storage.SQLiteBackend and a real cbl.Parse -> storage.ImportCBL /
// storage.ReimportCBL pipeline, since that's what comic-server-zw0o added
// and nothing had exercised against device sync planning before.
func TestComputeSyncPlan_CBLImportedReadingList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer sb.Close()

	batman := library.ComicBook{ID: "book-batman", FilePath: "/x/batman1.cbz", Series: "Batman", Number: "1"}
	detective := library.ComicBook{ID: "book-detective", FilePath: "/x/det27.cbz", Series: "Detective Comics", Number: "27"}
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{batman, detective}}, storage.ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}

	rl := &cbl.ReadingList{Name: "Unread", Books: []cbl.Book{
		{Series: "Batman", Number: "1"},
	}}
	importResult, err := sb.DB().ImportCBL(rl, storage.CBLImportSource{Source: "local_file"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}

	list, err := sb.FindListByID(importResult.ListID)
	if err != nil || list == nil {
		t.Fatalf("FindListByID: list=%v err=%v", list, err)
	}

	syncer := NewSyncer(&MockClient{}, sb)
	if err := syncer.SetFilterList(list); err != nil {
		t.Fatalf("SetFilterList: %v", err)
	}

	ops, err := syncer.ComputeSyncPlan(map[string]*DeviceBook{})
	if err != nil {
		t.Fatalf("ComputeSyncPlan (first): %v", err)
	}
	if len(ops) != 1 || ops[0].Book.ID != "book-batman" {
		t.Fatalf("first plan: ops = %+v, want exactly [book-batman]", ops)
	}

	// Reimport with an upstream CBL that now also includes Detective
	// Comics - proves the watch/reimport engine's full-replace
	// (comic-server-zw0o) actually changes the NEXT device sync plan,
	// not just what's in the database.
	updatedRL := &cbl.ReadingList{Name: "Unread", Books: []cbl.Book{
		{Series: "Batman", Number: "1"},
		{Series: "Detective Comics", Number: "27"},
	}}
	if _, err := sb.DB().ReimportCBL(importResult.ListID, updatedRL, storage.CBLImportSource{Source: "local_file"}); err != nil {
		t.Fatalf("ReimportCBL: %v", err)
	}

	// Re-fetch and re-set the filter list - ComputeSyncPlan resolves
	// membership through the list object SetFilterList captured, and a
	// reimport replaced that list's rows out from under the earlier
	// snapshot's Items slice.
	list, err = sb.FindListByID(importResult.ListID)
	if err != nil || list == nil {
		t.Fatalf("FindListByID (after reimport): list=%v err=%v", list, err)
	}
	if err := syncer.SetFilterList(list); err != nil {
		t.Fatalf("SetFilterList (after reimport): %v", err)
	}

	ops, err = syncer.ComputeSyncPlan(map[string]*DeviceBook{})
	if err != nil {
		t.Fatalf("ComputeSyncPlan (second): %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("second plan: ops = %+v, want 2 (Batman + Detective Comics after reimport)", ops)
	}
}
