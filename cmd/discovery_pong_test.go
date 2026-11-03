package cmd

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/syncstate"
)

// recordPongs swaps pongSender for a recorder for the length of the test.
func recordPongs(t *testing.T) *[]string {
	t.Helper()
	var ips []string
	old := pongSender
	pongSender = func(ip string) error {
		ips = append(ips, ip)
		return nil
	}
	t.Cleanup(func() { pongSender = old })
	return &ips
}

// pingDevice runs one discovery ping from a device the registry already
// knows, the way the multicast listener does.
func pingKnownDevice(t *testing.T, registered, wantsSync bool) (pongs *[]string, sm *syncstate.Manager) {
	t.Helper()
	pongs = recordPongs(t)
	sm = syncstate.NewManager(10)

	info := &device.Info{ID: "d1", Name: "Tab"}
	registry := device.NewRegistry()
	registry.Add(info, "192.168.0.171")

	db := newTestConfigDB(t)
	if registered {
		if err := db.UpsertDevice("d1", "Tab", time.Time{}, nil); err != nil {
			t.Fatal(err)
		}
	}

	handleDiscoveredDevice(
		device.DiscoveredDevice{Key: "d1", IPAddress: "192.168.0.171", WantsSync: wantsSync},
		registry, sm, &config.Config{}, nil, db, nil, nil, make(chan struct{}, 1),
	)
	return pongs, sm
}

// Tracker #55: an unregistered device must be answered, or the new app keeps
// saying it cannot find a server. Nothing about it may start a sync.
func TestHandleDiscoveredDevice_UnregisteredPingGetsPong(t *testing.T) {
	pongs, sm := pingKnownDevice(t, false, false)

	if len(*pongs) != 1 || (*pongs)[0] != "192.168.0.171" {
		t.Fatalf("pongs = %v, want one to 192.168.0.171", *pongs)
	}
	if sm.IsDeviceSyncing("d1") || len(sm.GetHistory(10)) != 0 {
		t.Error("a pong must not start a sync or leave a sync record")
	}
}

func TestHandleDiscoveredDevice_RegisteredPingStillGetsPong(t *testing.T) {
	pongs, _ := pingKnownDevice(t, true, false)
	if len(*pongs) != 1 {
		t.Fatalf("pongs = %v, want one", *pongs)
	}
}

// A sync request from an unregistered device is still refused: no pong, no
// sync, and the refusal is recorded.
func TestHandleDiscoveredDevice_UnregisteredSyncRequestStillRefused(t *testing.T) {
	pongs, sm := pingKnownDevice(t, false, true)

	if len(*pongs) != 0 {
		t.Errorf("a sync request is not answered with a pong, got %v", *pongs)
	}
	if sm.IsDeviceSyncing("d1") {
		t.Fatal("an unregistered device must not be syncing")
	}
	history := sm.GetHistory(10)
	if len(history) != 1 || history[0].Status != syncstate.StatusFailed ||
		history[0].ErrorMessage != errDeviceNotRegistered.Error() {
		t.Errorf("history = %+v, want one failed sync: %v", history, errDeviceNotRegistered)
	}
}

func TestAnswerDevice_PongFailureIsNotAnError(t *testing.T) {
	var calls atomic.Int32
	old := pongSender
	pongSender = func(string) error { calls.Add(1); return device.ErrNotConnected }
	t.Cleanup(func() { pongSender = old })

	answerDevice(newTestConfigDB(t), "d9", "10.0.0.9")
	if calls.Load() != 1 {
		t.Errorf("pong attempts = %d, want 1", calls.Load())
	}
}
