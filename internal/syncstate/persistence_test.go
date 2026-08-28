package syncstate

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
)

func newTestStore(t *testing.T) *configdb.DB {
	t.Helper()
	db, err := configdb.Open(t.TempDir() + "/config.db")
	if err != nil {
		t.Fatalf("failed to open test config db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNewManagerWithStore_PersistsCompletedSync(t *testing.T) {
	store := newTestStore(t)

	m, err := NewManagerWithStore(10, store)
	if err != nil {
		t.Fatalf("NewManagerWithStore failed: %v", err)
	}

	if err := m.StartSync("device1", "192.168.1.100", "My Tablet"); err != nil {
		t.Fatalf("StartSync failed: %v", err)
	}
	m.CompleteSync("device1", 3, 2, 1, 0)

	history := m.GetHistory(0)
	if len(history) != 1 {
		t.Fatalf("expected 1 in-memory history entry, got %d", len(history))
	}

	records, err := store.LoadRecentSyncHistory(10)
	if err != nil {
		t.Fatalf("LoadRecentSyncHistory failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 persisted record, got %d", len(records))
	}
	if records[0].DeviceID != "device1" || records[0].BooksAdded != 3 || records[0].Status != string(StatusCompleted) {
		t.Errorf("unexpected persisted record: %+v", records[0])
	}
}

func TestNewManagerWithStore_WarmsFromExistingHistory(t *testing.T) {
	store := newTestStore(t)

	m1, err := NewManagerWithStore(10, store)
	if err != nil {
		t.Fatalf("NewManagerWithStore failed: %v", err)
	}
	if err := m1.StartSync("device1", "192.168.1.100", "My Tablet"); err != nil {
		t.Fatalf("StartSync failed: %v", err)
	}
	m1.CompleteSync("device1", 1, 0, 0, 0)

	// Simulate a restart: a fresh Manager backed by the same store should
	// come back up with the prior sync already in its in-memory history.
	m2, err := NewManagerWithStore(10, store)
	if err != nil {
		t.Fatalf("NewManagerWithStore (restart) failed: %v", err)
	}

	history := m2.GetHistory(0)
	if len(history) != 1 {
		t.Fatalf("expected history to survive restart, got %d entries", len(history))
	}
	if history[0].DeviceID != "device1" {
		t.Errorf("unexpected restored history entry: %+v", history[0])
	}
}
