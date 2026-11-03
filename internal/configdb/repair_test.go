package configdb

import (
	"errors"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/sync"
)

func seedHistory(t *testing.T, db *DB, deviceID, status string, added int, msg string) {
	t.Helper()
	end := time.Now()
	err := db.AppendSyncHistory(SyncHistoryRecord{
		DeviceID: deviceID, DeviceName: "Tab", StartTime: end.Add(-time.Minute), EndTime: &end,
		Status: status, BooksAdded: added, ErrorMessage: msg,
	}, 100)
	if err != nil {
		t.Fatalf("seed history: %v", err)
	}
}

func historyCount(t *testing.T, db *DB, deviceID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_history WHERE device_id = ?`, deviceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seedOldDevice registers "old" with settings, an address, two lists (one
// disabled, one with its own settings) and some history.
func seedOldDevice(t *testing.T, db *DB) {
	t.Helper()
	defaults := &sync.SharedListSettings{OnlyUnread: true, Limit: true, LimitValue: 25}
	if err := db.UpsertDevice("old", "My Tablet", time.Now().Truncate(time.Second), defaults); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeviceAddress("old", "100.85.206.109"); err != nil {
		t.Fatal(err)
	}
	own := &sync.SharedListSettings{Limit: true, LimitValue: 5}
	if err := db.AddDeviceList("old", DeviceList{ListID: "l1", ListName: "One", Enabled: true, Settings: own}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddDeviceList("old", DeviceList{ListID: "l2", ListName: "Two", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	seedHistory(t, db, "old", "completed", 21, "")
	seedHistory(t, db, "old", "failed", 0, "boom")
}

func TestRepairDevice_MovesEverything(t *testing.T) {
	db := newTestDB(t)
	seedOldDevice(t, db)

	if err := db.RepairDevice("old", "new"); err != nil {
		t.Fatalf("RepairDevice: %v", err)
	}

	if d, _ := db.GetDevice("old"); d != nil {
		t.Error("the old ID should be gone")
	}
	d, err := db.GetDevice("new")
	if err != nil || d == nil {
		t.Fatalf("GetDevice(new) = %v, %v", d, err)
	}
	if d.FriendlyName != "My Tablet" || d.Address != "100.85.206.109" {
		t.Errorf("name/address = %q / %q", d.FriendlyName, d.Address)
	}
	if d.DefaultSettings == nil || d.DefaultSettings.LimitValue != 25 || !d.DefaultSettings.OnlyUnread {
		t.Errorf("default settings = %+v", d.DefaultSettings)
	}
	if len(d.Lists) != 2 {
		t.Fatalf("lists = %+v, want 2", d.Lists)
	}
	for _, l := range d.Lists {
		switch l.ListID {
		case "l1":
			if !l.Enabled || l.Settings == nil || l.Settings.LimitValue != 5 {
				t.Errorf("l1 = %+v, want enabled with its own settings", l)
			}
		case "l2":
			if l.Enabled || l.Settings != nil {
				t.Errorf("l2 = %+v, want disabled and inheriting", l)
			}
		}
	}
	if n := historyCount(t, db, "new"); n != 2 {
		t.Errorf("history on new = %d, want 2", n)
	}
	if n := historyCount(t, db, "old"); n != 0 {
		t.Errorf("history left on old = %d, want 0", n)
	}
}

func TestRepairDevice_Refusals(t *testing.T) {
	db := newTestDB(t)
	seedOldDevice(t, db)

	if err := db.RepairDevice("old", "old"); !errors.Is(err, ErrRepairSameDevice) {
		t.Errorf("same ID: %v", err)
	}
	if err := db.RepairDevice("ghost", "new"); !errors.Is(err, ErrRepairNotRegistered) {
		t.Errorf("unregistered old: %v", err)
	}
}

func TestRepairDevice_TargetWithListsOrRealHistoryIsRefusedAndNothingMoves(t *testing.T) {
	cases := map[string]func(t *testing.T, db *DB){
		"has a list": func(t *testing.T, db *DB) {
			if err := db.UpsertDevice("new", "X", time.Time{}, nil); err != nil {
				t.Fatal(err)
			}
			if err := db.AddDeviceList("new", DeviceList{ListID: "lx", Enabled: true}); err != nil {
				t.Fatal(err)
			}
		},
		"has a completed sync": func(t *testing.T, db *DB) {
			if err := db.UpsertDevice("new", "X", time.Time{}, nil); err != nil {
				t.Fatal(err)
			}
			seedHistory(t, db, "new", "completed", 0, "")
		},
		"has a failed sync that changed books": func(t *testing.T, db *DB) {
			seedHistory(t, db, "new", "failed", 3, "dropped")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			db := newTestDB(t)
			seedOldDevice(t, db)
			setup(t, db)

			if err := db.RepairDevice("old", "new"); !errors.Is(err, ErrRepairTargetInUse) {
				t.Fatalf("err = %v, want ErrRepairTargetInUse", err)
			}
			d, _ := db.GetDevice("old")
			if d == nil || len(d.Lists) != 2 || historyCount(t, db, "old") != 2 {
				t.Errorf("old must be untouched, got %+v", d)
			}
		})
	}
}

// The common case: the user pressed Register on the new device, so it has an
// empty entry, and its failed "not registered" sync attempts are on record.
func TestRepairDevice_ReplacesAnEmptyRegisteredTarget(t *testing.T) {
	db := newTestDB(t)
	seedOldDevice(t, db)
	if err := db.UpsertDevice("new", "Fresh name", time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	seedHistory(t, db, "new", "failed", 0, "device is not registered; register it and assign lists before syncing")

	if err := db.RepairDevice("old", "new"); err != nil {
		t.Fatalf("RepairDevice: %v", err)
	}
	d, _ := db.GetDevice("new")
	if d == nil || d.FriendlyName != "My Tablet" || len(d.Lists) != 2 {
		t.Errorf("new = %+v, want the old device's name and lists", d)
	}
	// 2 moved + 1 earlier failed attempt, which is accurate and stays.
	if n := historyCount(t, db, "new"); n != 3 {
		t.Errorf("history on new = %d, want 3", n)
	}
}

// If any step fails, everything stays on the old ID.
func TestRepairDevice_IsAtomic(t *testing.T) {
	db := newTestDB(t)
	seedOldDevice(t, db)
	if _, err := db.Exec(`CREATE TRIGGER boom BEFORE UPDATE ON sync_history
		BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}

	if err := db.RepairDevice("old", "new"); err == nil {
		t.Fatal("expected the failing history update to fail the re-pair")
	}
	if d, _ := db.GetDevice("new"); d != nil {
		t.Errorf("new must not exist after a failed re-pair, got %+v", d)
	}
	d, _ := db.GetDevice("old")
	if d == nil || len(d.Lists) != 2 || historyCount(t, db, "old") != 2 {
		t.Errorf("old must be intact, got %+v", d)
	}
}

func TestRepairDevice_CanBeUndone(t *testing.T) {
	db := newTestDB(t)
	seedOldDevice(t, db)
	if err := db.RepairDevice("old", "new"); err != nil {
		t.Fatal(err)
	}
	if err := db.RepairDevice("new", "old"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	d, _ := db.GetDevice("old")
	if d == nil || len(d.Lists) != 2 || historyCount(t, db, "old") != 2 {
		t.Errorf("after undo old = %+v", d)
	}
}
