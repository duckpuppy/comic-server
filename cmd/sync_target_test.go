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
		// quick* describe the check of the old LAN address (ProbeQuick).
		quickRes  *device.ProbeResult
		quickErr  error
		wantQuick bool
	}
	cases := []tc{
		{name: "recent ping beats the manual address", registered: true, age: 10 * time.Second, address: "100.85.206.109", probeRes: okProbe, wantIP: "192.168.0.171", wantProbed: false},
		{name: "stale ping uses the manual address", registered: true, age: 10 * time.Minute, address: "100.85.206.109", probeRes: okProbe, wantIP: "100.85.206.109", wantVia: true, wantQuick: true, wantProbed: true},
		{name: "never seen uses the manual address", address: "tablet.ts.net", probeRes: okProbe, wantIP: "tablet.ts.net", wantVia: true, wantProbed: true},
		{name: "stale ping and no address keeps the old behaviour", registered: true, age: 10 * time.Minute, wantIP: "192.168.0.171"},
		{name: "nothing known", wantErr: device.ErrNotConnected},
		{name: "wrong device at the address is an error, not a fallback", registered: true, age: 10 * time.Minute, address: "100.1.1.1", probeErr: device.ErrAddressMismatch, wantErr: device.ErrAddressMismatch, wantQuick: true, wantProbed: true},
		{name: "unreachable address falls back to the stale LAN address", registered: true, age: 10 * time.Minute, address: "100.1.1.1", probeErr: device.ErrProbeFailed, wantIP: "192.168.0.171", wantQuick: true, wantProbed: true},
		{name: "stale ping: the old LAN address is tried first and wins", registered: true, age: 10 * time.Minute, address: "100.85.206.109", quickRes: okProbe, wantIP: "192.168.0.171", wantQuick: true, wantProbed: false},
		{name: "stale ping: dead LAN address falls through to the manual address", registered: true, age: 10 * time.Minute, address: "100.85.206.109", quickErr: device.ErrProbeFailed, probeRes: okProbe, wantIP: "100.85.206.109", wantVia: true, wantQuick: true, wantProbed: true},
		{name: "stale ping: another device on the old LAN address is skipped, not an error", registered: true, age: 10 * time.Minute, address: "100.85.206.109", quickErr: device.ErrAddressMismatch, probeRes: okProbe, wantIP: "100.85.206.109", wantVia: true, wantQuick: true, wantProbed: true},
		{name: "no manual address: no quick check", registered: true, age: 10 * time.Minute, wantIP: "192.168.0.171", wantQuick: false},
		{name: "never seen: no quick check", address: "tablet.ts.net", probeRes: okProbe, wantIP: "tablet.ts.net", wantVia: true, wantQuick: false, wantProbed: true},
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

			// Unless a case says otherwise, the old LAN address does not answer.
			quickErr := c.quickErr
			if c.quickRes == nil && quickErr == nil {
				quickErr = device.ErrProbeFailed
			}
			calls, quickCalls := 0, 0
			got, err := resolveSyncTarget("d1", registry, db, now,
				fakeProbe(c.probeRes, c.probeErr, &calls),
				fakeProbe(c.quickRes, quickErr, &quickCalls))
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
			if (quickCalls > 0) != c.wantQuick {
				t.Errorf("quick probed = %v, want %v", quickCalls > 0, c.wantQuick)
			}
		})
	}
}
