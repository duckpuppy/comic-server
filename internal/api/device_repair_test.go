package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/device"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/syncstate"
	ws "github.com/duckpuppy/comic-server/internal/websocket"
)

func newRepairTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("open config db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertDevice("old-id", "My Tablet", time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.AddDeviceList("old-id", configdb.DeviceList{ListID: "l1", ListName: "One", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	reg := device.NewRegistry()
	reg.Add(&device.Info{ID: "old-id", Name: "My Tablet"}, "192.168.0.171")
	reg.Add(&device.Info{ID: "new-id", Name: "My Tablet"}, "192.168.0.171")

	return &Server{
		registry:          reg,
		configDB:          db,
		listCache:         library.NewListCache(time.Minute),
		syncManager:       syncstate.NewManager(100),
		registeredDevices: map[string]bool{"old-id": true},
		wsHub:             ws.NewHub(),
	}
}

func repairRequest(s *Server, oldID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/devices/"+oldID+"/repair", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleDevicesRouter(w, req)
	return w
}

func TestRepairDevice_API_MovesAndUpdatesMemory(t *testing.T) {
	s := newRepairTestServer(t)
	s.syncManager.StartSync("old-id", "192.168.0.171", "My Tablet")
	s.syncManager.CompleteSync("old-id", 1, 0, 0, 0)

	w := repairRequest(s, "old-id", `{"new_device_id":" new-id "}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}

	if d, _ := s.configDB.GetDevice("new-id"); d == nil || len(d.Lists) != 1 {
		t.Errorf("new-id = %+v, want 1 list", d)
	}
	if d, _ := s.configDB.GetDevice("old-id"); d != nil {
		t.Error("old-id should be gone")
	}
	if s.registeredDevices["old-id"] || !s.registeredDevices["new-id"] {
		t.Errorf("registeredDevices = %v", s.registeredDevices)
	}
	if _, ok := s.registry.Get("old-id"); ok {
		t.Error("old-id should leave the live registry")
	}
	if h, _ := s.syncManager.GetHistoryForDevice("new-id", 10, 0); len(h) != 1 {
		t.Errorf("in-memory history on new-id = %d, want 1", len(h))
	}
	if h, _ := s.syncManager.GetHistoryForDevice("old-id", 10, 0); len(h) != 0 {
		t.Errorf("in-memory history left on old-id = %d, want 0", len(h))
	}
}

func TestRepairDevice_API_Errors(t *testing.T) {
	cases := []struct {
		name string
		old  string
		body string
		want int
	}{
		{"bad JSON", "old-id", `not json`, http.StatusBadRequest},
		{"missing new id", "old-id", `{}`, http.StatusBadRequest},
		{"new id with a slash", "old-id", `{"new_device_id":"a/b"}`, http.StatusBadRequest},
		{"same id", "old-id", `{"new_device_id":"old-id"}`, http.StatusBadRequest},
		{"unknown old id", "ghost", `{"new_device_id":"new-id"}`, http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newRepairTestServer(t)
			if w := repairRequest(s, c.old, c.body); w.Code != c.want {
				t.Errorf("status = %d, want %d: %s", w.Code, c.want, w.Body)
			}
		})
	}
}

func TestRepairDevice_API_RefusesWhileSyncing(t *testing.T) {
	s := newRepairTestServer(t)
	if err := s.syncManager.StartSync("old-id", "192.168.0.171", "My Tablet"); err != nil {
		t.Fatal(err)
	}

	if w := repairRequest(s, "old-id", `{"new_device_id":"new-id"}`); w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
	if d, _ := s.configDB.GetDevice("old-id"); d == nil {
		t.Error("nothing may move while a sync runs")
	}
}

func TestRepairDevice_API_TargetInUseIs409(t *testing.T) {
	s := newRepairTestServer(t)
	if err := s.configDB.UpsertDevice("new-id", "X", time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.configDB.AddDeviceList("new-id", configdb.DeviceList{ListID: "lx", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if w := repairRequest(s, "old-id", `{"new_device_id":"new-id"}`); w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", w.Code, w.Body)
	}
}

func TestRepairDevice_API_GetNotAllowed(t *testing.T) {
	s := newRepairTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/devices/old-id/repair", nil)
	w := httptest.NewRecorder()
	s.handleDevicesRouter(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
