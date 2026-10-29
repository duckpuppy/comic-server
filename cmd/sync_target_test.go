package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/device"
)

// fakeProbe returns a probe that answers with a canned result.
func fakeProbe(res *device.ProbeResult, err error, calls *int) func(string, string) (*device.ProbeResult, error) {
	return func(string, string) (*device.ProbeResult, error) {
		if calls != nil {
			*calls++
		}
		return res, err
	}
}

func TestResolveSyncTarget(t *testing.T) {
	now := time.Now()
	lanInfo := &device.Info{ID: "d1", Name: "LAN"}
	tsInfo := &device.Info{ID: "d1", Name: "Probed"}
	okProbe := &device.ProbeResult{Info: tsInfo}

	type tc struct {
		name       string
		registered bool          // device is in the registry
		age        time.Duration // time since its last ping
		address    string        // manual address in config.db
		probeRes   *device.ProbeResult
		probeErr   error
		wantIP     string
		wantVia    bool
		wantErr    error
		wantProbed bool
	}
	cases := []tc{
		{name: "recent ping beats the manual address", registered: true, age: 10 * time.Second, address: "100.85.206.109", probeRes: okProbe, wantIP: "192.168.0.171", wantProbed: false},
		{name: "stale ping uses the manual address", registered: true, age: 10 * time.Minute, address: "100.85.206.109", probeRes: okProbe, wantIP: "100.85.206.109", wantVia: true, wantProbed: true},
		{name: "never seen uses the manual address", address: "tablet.ts.net", probeRes: okProbe, wantIP: "tablet.ts.net", wantVia: true, wantProbed: true},
		{name: "stale ping and no address keeps the old behaviour", registered: true, age: 10 * time.Minute, wantIP: "192.168.0.171"},
		{name: "nothing known", wantErr: device.ErrNotConnected},
		{name: "wrong device at the address is an error, not a fallback", registered: true, age: 10 * time.Minute, address: "100.1.1.1", probeErr: device.ErrAddressMismatch, wantErr: device.ErrAddressMismatch, wantProbed: true},
		{name: "unreachable address falls back to the stale LAN address", registered: true, age: 10 * time.Minute, address: "100.1.1.1", probeErr: device.ErrProbeFailed, wantIP: "192.168.0.171", wantProbed: true},
		{name: "unreachable address and never seen is an error", address: "100.1.1.1", probeErr: device.ErrProbeFailed, wantErr: device.ErrProbeFailed, wantProbed: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			registry := device.NewRegistry()
			if c.registered {
				registry.Add(lanInfo, "192.168.0.171")
				dev, _ := registry.Get("d1")
				dev.LastSeen = now.Add(-c.age)
			}
			db := newTestConfigDB(t)
			if err := db.UpsertDevice("d1", "Tab", time.Time{}, nil); err != nil {
				t.Fatal(err)
			}
			if c.address != "" {
				if err := db.SetDeviceAddress("d1", c.address); err != nil {
					t.Fatal(err)
				}
			}

			calls := 0
			got, err := resolveSyncTarget("d1", registry, db, now, fakeProbe(c.probeRes, c.probeErr, &calls))
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else {
				if got.ip != c.wantIP || got.viaAddress != c.wantVia {
					t.Errorf("target = %q via=%v, want %q via=%v", got.ip, got.viaAddress, c.wantIP, c.wantVia)
				}
			}
			if (calls > 0) != c.wantProbed {
				t.Errorf("probed = %v, want %v", calls > 0, c.wantProbed)
			}
		})
	}
}
