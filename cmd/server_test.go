package cmd

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	csync "github.com/duckpuppy/comic-server/internal/sync"
)

// TestApplyDeviceConfig_AcceptsIdList is the regression test for the most
// severe symptom of comic-server-vwl's device-sync bug: applyDeviceConfig
// used to hard-reject any assigned list whose Type didn't contain
// "SmartList" ("smart list %s (ID: %s) not found in library"), and that
// error aborts handleSyncRequest entirely - meaning a device with even one
// real ID list assigned (e.g. "To Read") had its WHOLE sync fail, not just
// that one list. Confirmed against ComicRackCE's own source that "devices
// only sync smart lists" isn't a real protocol constraint - any list type
// with real book membership should be accepted; only folders should still
// be rejected.
func TestApplyDeviceConfig_AcceptsIdList(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1"},
		},
		ComicLists: []library.ComicListItem{
			{ID: "idlist-1", Name: "To Read", Type: "ComicIdListItem", BookIds: []string{"book1"}},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := csync.NewSyncer(nil, backend)

	deviceConfig := &configdb.Device{
		DeviceID: "device-1",
		Lists: []configdb.DeviceList{
			{ListID: "idlist-1", ListName: "To Read", Enabled: true},
		},
	}

	if err := applyDeviceConfig(syncer, deviceConfig, backend); err != nil {
		t.Fatalf("expected an ID list to be accepted, got error: %v", err)
	}
}

// TestApplyDeviceConfig_RejectsFolder confirms a folder (which groups
// other lists rather than containing books itself) is still rejected,
// rather than the fix above having simply removed all validation.
func TestApplyDeviceConfig_RejectsFolder(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{ID: "folder-1", Name: "A Folder", Type: "ComicListItemFolder"},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := csync.NewSyncer(nil, backend)

	deviceConfig := &configdb.Device{
		DeviceID: "device-1",
		Lists: []configdb.DeviceList{
			{ListID: "folder-1", ListName: "A Folder", Enabled: true},
		},
	}

	if err := applyDeviceConfig(syncer, deviceConfig, backend); err == nil {
		t.Error("expected an error assigning a folder, got nil")
	}
}
