package syncstate

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager(50)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.maxHistory != 50 {
		t.Errorf("expected maxHistory 50, got %d", m.maxHistory)
	}

	// Test default maxHistory
	m2 := NewManager(0)
	if m2.maxHistory != 100 {
		t.Errorf("expected default maxHistory 100, got %d", m2.maxHistory)
	}
}

func TestStartSync(t *testing.T) {
	m := NewManager(10)

	err := m.StartSync("device1", "192.168.1.100", "My Tablet")
	if err != nil {
		t.Fatalf("StartSync failed: %v", err)
	}

	// Verify sync state was created
	state, exists := m.GetActiveSync("device1")
	if !exists {
		t.Fatal("sync state not found")
	}
	if state.DeviceID != "device1" {
		t.Errorf("expected deviceID 'device1', got '%s'", state.DeviceID)
	}
	if state.Status != StatusStarting {
		t.Errorf("expected status Starting, got %s", state.Status)
	}

	// Try to start sync for same device again
	err = m.StartSync("device1", "192.168.1.100", "My Tablet")
	if err == nil {
		t.Error("expected error when starting sync for already-syncing device")
	}
	if _, ok := err.(*DeviceAlreadySyncingError); !ok {
		t.Errorf("expected DeviceAlreadySyncingError, got %T", err)
	}
}

func TestUpdateProgress(t *testing.T) {
	m := NewManager(10)
	m.StartSync("device1", "192.168.1.100", "My Tablet")

	m.UpdateProgress("device1", 50, 100, 10, 5, 2, 1)

	state, exists := m.GetActiveSync("device1")
	if !exists {
		t.Fatal("sync state not found")
	}
	if state.Progress != 50 {
		t.Errorf("expected progress 50, got %d", state.Progress)
	}
	if state.BooksTotal != 100 {
		t.Errorf("expected booksTotal 100, got %d", state.BooksTotal)
	}
	if state.BooksAdded != 10 {
		t.Errorf("expected booksAdded 10, got %d", state.BooksAdded)
	}
	if state.Status != StatusInProgress {
		t.Errorf("expected status InProgress, got %s", state.Status)
	}
}

func TestCompleteSync(t *testing.T) {
	m := NewManager(10)
	m.StartSync("device1", "192.168.1.100", "My Tablet")
	m.UpdateProgress("device1", 50, 100, 10, 5, 2, 1)

	m.CompleteSync("device1", 25, 10, 5, 2)

	// Should no longer be in active syncs
	_, exists := m.GetActiveSync("device1")
	if exists {
		t.Error("sync should not be in active syncs after completion")
	}

	// Should be in history
	history := m.GetHistory(10)
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].DeviceID != "device1" {
		t.Errorf("expected deviceID 'device1', got '%s'", history[0].DeviceID)
	}
	if history[0].Status != StatusCompleted {
		t.Errorf("expected status Completed, got %s", history[0].Status)
	}
	if history[0].BooksAdded != 25 {
		t.Errorf("expected booksAdded 25, got %d", history[0].BooksAdded)
	}
	if history[0].EndTime == nil {
		t.Error("expected EndTime to be set")
	}
}

func TestFailSync(t *testing.T) {
	m := NewManager(10)
	m.StartSync("device1", "192.168.1.100", "My Tablet")

	m.FailSync("device1", "connection timeout")

	// Should not be in active syncs
	_, exists := m.GetActiveSync("device1")
	if exists {
		t.Error("sync should not be in active syncs after failure")
	}

	// Should be in history with error
	history := m.GetHistory(10)
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Status != StatusFailed {
		t.Errorf("expected status Failed, got %s", history[0].Status)
	}
	if history[0].ErrorMessage != "connection timeout" {
		t.Errorf("expected error message 'connection timeout', got '%s'", history[0].ErrorMessage)
	}
}

func TestAbortSync(t *testing.T) {
	m := NewManager(10)
	m.StartSync("device1", "192.168.1.100", "My Tablet")

	m.AbortSync("device1")

	// Should not be in active syncs
	_, exists := m.GetActiveSync("device1")
	if exists {
		t.Error("sync should not be in active syncs after abort")
	}

	// Should be in history
	history := m.GetHistory(10)
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Status != StatusAborted {
		t.Errorf("expected status Aborted, got %s", history[0].Status)
	}
}

func TestGetAllActiveSyncs(t *testing.T) {
	m := NewManager(10)

	m.StartSync("device1", "192.168.1.100", "Tablet 1")
	m.StartSync("device2", "192.168.1.101", "Tablet 2")
	m.StartSync("device3", "192.168.1.102", "Tablet 3")

	syncs := m.GetAllActiveSyncs()
	if len(syncs) != 3 {
		t.Fatalf("expected 3 active syncs, got %d", len(syncs))
	}

	// Verify all devices are present
	deviceIDs := make(map[string]bool)
	for _, sync := range syncs {
		deviceIDs[sync.DeviceID] = true
	}
	if !deviceIDs["device1"] || !deviceIDs["device2"] || !deviceIDs["device3"] {
		t.Error("not all devices found in active syncs")
	}
}

func TestIsDeviceSyncing(t *testing.T) {
	m := NewManager(10)

	if m.IsDeviceSyncing("device1") {
		t.Error("device1 should not be syncing initially")
	}

	m.StartSync("device1", "192.168.1.100", "My Tablet")

	if !m.IsDeviceSyncing("device1") {
		t.Error("device1 should be syncing after StartSync")
	}

	m.CompleteSync("device1", 10, 5, 2, 0)

	if m.IsDeviceSyncing("device1") {
		t.Error("device1 should not be syncing after completion")
	}
}

func TestGetActiveCount(t *testing.T) {
	m := NewManager(10)

	if m.GetActiveCount() != 0 {
		t.Errorf("expected 0 active syncs, got %d", m.GetActiveCount())
	}

	m.StartSync("device1", "192.168.1.100", "Tablet 1")
	m.StartSync("device2", "192.168.1.101", "Tablet 2")

	if m.GetActiveCount() != 2 {
		t.Errorf("expected 2 active syncs, got %d", m.GetActiveCount())
	}

	m.CompleteSync("device1", 10, 5, 2, 0)

	if m.GetActiveCount() != 1 {
		t.Errorf("expected 1 active sync, got %d", m.GetActiveCount())
	}
}

func TestHistoryLimit(t *testing.T) {
	m := NewManager(3) // Small history for testing

	// Complete 5 syncs
	for i := 1; i <= 5; i++ {
		deviceID := "device" + string(rune('0'+i))
		m.StartSync(deviceID, "192.168.1.100", "Tablet")
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
		m.CompleteSync(deviceID, i, i, i, 0)
	}

	// Should only keep last 3
	history := m.GetHistory(10)
	if len(history) != 3 {
		t.Fatalf("expected history length 3, got %d", len(history))
	}

	// Verify it's the last 3 (device3, device4, device5)
	expectedDevices := []string{"device3", "device4", "device5"}
	for i, state := range history {
		if state.DeviceID != expectedDevices[i] {
			t.Errorf("expected device %s at position %d, got %s", expectedDevices[i], i, state.DeviceID)
		}
	}
}

func TestGetHistoryLimit(t *testing.T) {
	m := NewManager(100)

	// Complete 10 syncs
	for i := 1; i <= 10; i++ {
		deviceID := "device" + string(rune('0'+i))
		m.StartSync(deviceID, "192.168.1.100", "Tablet")
		m.CompleteSync(deviceID, i, i, i, 0)
	}

	// Get last 5
	history := m.GetHistory(5)
	if len(history) != 5 {
		t.Fatalf("expected 5 history entries, got %d", len(history))
	}

	// Verify it's the last 5
	expectedStart := 6
	for i, state := range history {
		expectedID := "device" + string(rune('0'+expectedStart+i))
		if state.DeviceID != expectedID {
			t.Errorf("expected device %s at position %d, got %s", expectedID, i, state.DeviceID)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager(100)

	done := make(chan bool)

	// Spawn multiple goroutines
	for i := 0; i < 10; i++ {
		go func(id int) {
			deviceID := "device" + string(rune('0'+id))
			m.StartSync(deviceID, "192.168.1.100", "Tablet")
			m.UpdateProgress(deviceID, 50, 100, 10, 5, 2, 0)
			m.CompleteSync(deviceID, 25, 10, 5, 0)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 10 history entries
	history := m.GetHistory(100)
	if len(history) != 10 {
		t.Errorf("expected 10 history entries, got %d", len(history))
	}

	// No active syncs
	if m.GetActiveCount() != 0 {
		t.Errorf("expected 0 active syncs, got %d", m.GetActiveCount())
	}
}
