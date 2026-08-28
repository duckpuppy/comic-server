package configdb

import (
	"testing"
	"time"
)

func TestSyncHistory_AppendAndLoad(t *testing.T) {
	db := newTestDB(t)

	base := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		end := base.Add(time.Duration(i) * time.Minute)
		rec := SyncHistoryRecord{
			DeviceID:     "device-1",
			DeviceIP:     "192.168.0.1",
			DeviceName:   "Test Tablet",
			StartTime:    base.Add(-time.Duration(i) * time.Minute),
			EndTime:      &end,
			Status:       "completed",
			Progress:     100,
			BooksTotal:   10,
			BooksAdded:   1,
			BooksUpdated: 2,
			BooksDeleted: 3,
			ErrorCount:   0,
			ErrorMessage: "",
		}
		if err := db.AppendSyncHistory(rec, 100); err != nil {
			t.Fatalf("AppendSyncHistory failed: %v", err)
		}
	}

	records, err := db.LoadRecentSyncHistory(100)
	if err != nil {
		t.Fatalf("LoadRecentSyncHistory failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	// Oldest-inserted first: the first insert (i=0) should come first, the
	// last insert (i=2) last.
	if !records[0].StartTime.Equal(base) {
		t.Errorf("records[0].StartTime = %v, want first inserted (%v)", records[0].StartTime, base)
	}
	if !records[2].StartTime.Equal(base.Add(-2 * time.Minute)) {
		t.Errorf("records[2].StartTime = %v, want last inserted (%v)", records[2].StartTime, base.Add(-2*time.Minute))
	}
	if records[0].DeviceID != "device-1" || records[0].EndTime == nil {
		t.Errorf("unexpected record: %+v", records[0])
	}
}

func TestSyncHistory_AppendPrunesToKeep(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < 5; i++ {
		rec := SyncHistoryRecord{
			DeviceID:  "device-1",
			StartTime: time.Now(),
			Status:    "completed",
		}
		if err := db.AppendSyncHistory(rec, 3); err != nil {
			t.Fatalf("AppendSyncHistory failed: %v", err)
		}
	}

	records, err := db.LoadRecentSyncHistory(100)
	if err != nil {
		t.Fatalf("LoadRecentSyncHistory failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records after pruning, want 3 (keep cap)", len(records))
	}
}

func TestSyncHistory_LoadEmpty(t *testing.T) {
	db := newTestDB(t)

	records, err := db.LoadRecentSyncHistory(100)
	if err != nil {
		t.Fatalf("LoadRecentSyncHistory failed: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got %d records, want 0", len(records))
	}
}
