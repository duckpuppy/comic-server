package api

import (
	"encoding/json"
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
)

func newAddressTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("open config db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertDevice("dev-1", "Tab", time.Time{}, nil); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	return &Server{
		registry:    device.NewRegistry(),
		configDB:    db,
		listCache:   library.NewListCache(time.Minute),
		syncManager: syncstate.NewManager(100),
	}
}

func patchDevice(s *Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/api/devices/dev-1", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleDevicesRouter(w, req)
	return w
}

func TestPatchDevice_SavesAndClearsAddress(t *testing.T) {
	s := newAddressTestServer(t)

	if w := patchDevice(s, `{"address":" 100.85.206.109 "}`); w.Code != http.StatusOK {
		t.Fatalf("PATCH = %d: %s", w.Code, w.Body)
	}
	d, _ := s.configDB.GetDevice("dev-1")
	if d.Address != "100.85.206.109" {
		t.Errorf("saved Address = %q", d.Address)
	}

	// The detail endpoint returns it.
	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1", nil)
	w := httptest.NewRecorder()
	s.handleDevicesRouter(w, req)
	var detail DeviceDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Address != "100.85.206.109" {
		t.Errorf("detail Address = %q", detail.Address)
	}

	if w := patchDevice(s, `{"address":""}`); w.Code != http.StatusOK {
		t.Fatalf("clear PATCH = %d", w.Code)
	}
	if d, _ = s.configDB.GetDevice("dev-1"); d.Address != "" {
		t.Errorf("cleared Address = %q", d.Address)
	}
}

func TestPatchDevice_BadAddressSavesNothing(t *testing.T) {
	s := newAddressTestServer(t)
	w := patchDevice(s, `{"friendly_name":"Renamed","address":"host:7614"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PATCH = %d, want 400", w.Code)
	}
	d, _ := s.configDB.GetDevice("dev-1")
	if d.FriendlyName != "Tab" || d.Address != "" {
		t.Errorf("a bad address half-applied the update: %+v", d)
	}
}

func TestPatchDevice_OtherFieldsKeepAddress(t *testing.T) {
	s := newAddressTestServer(t)
	if err := s.configDB.SetDeviceAddress("dev-1", "tablet.ts.net"); err != nil {
		t.Fatal(err)
	}
	if w := patchDevice(s, `{"friendly_name":"New name"}`); w.Code != http.StatusOK {
		t.Fatalf("PATCH = %d", w.Code)
	}
	if d, _ := s.configDB.GetDevice("dev-1"); d.Address != "tablet.ts.net" {
		t.Errorf("a rename wiped the address: %q", d.Address)
	}
}

func TestTestDeviceAddress_BadRequests(t *testing.T) {
	s := newAddressTestServer(t)
	for name, body := range map[string]string{
		"no saved or typed address": ``,
		"invalid typed address":     `{"address":"bad host"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/devices/dev-1/test", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.handleDevicesRouter(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: POST = %d, want 400", name, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1/test", nil)
	w := httptest.NewRecorder()
	s.handleDevicesRouter(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d, want 405", w.Code)
	}
}
