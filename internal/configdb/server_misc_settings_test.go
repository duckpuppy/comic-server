package configdb

import (
	"reflect"
	"testing"
)

func TestServerMiscSettings_GetReturnsNilWhenUnset(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetServerMiscSettings()
	if err != nil {
		t.Fatalf("GetServerMiscSettings failed: %v", err)
	}
	if got != nil {
		t.Errorf("GetServerMiscSettings on empty db = %+v, want nil", got)
	}
}

func TestServerMiscSettings_UpsertAndGet(t *testing.T) {
	db := newTestDB(t)

	s := ServerMiscSettings{CBZConvertEnabled: true, IgnoreDevices: []string{"192.168.0.24", "SM-T970"}}
	if err := db.UpsertServerMiscSettings(s); err != nil {
		t.Fatalf("UpsertServerMiscSettings failed: %v", err)
	}

	got, err := db.GetServerMiscSettings()
	if err != nil {
		t.Fatalf("GetServerMiscSettings failed: %v", err)
	}
	if got == nil || got.CBZConvertEnabled != true || !reflect.DeepEqual(got.IgnoreDevices, s.IgnoreDevices) {
		t.Errorf("GetServerMiscSettings = %+v, want %+v", got, s)
	}

	// Upsert again overwrites wholesale, not merges.
	s2 := ServerMiscSettings{CBZConvertEnabled: false, IgnoreDevices: nil}
	if err := db.UpsertServerMiscSettings(s2); err != nil {
		t.Fatalf("second UpsertServerMiscSettings failed: %v", err)
	}
	got2, err := db.GetServerMiscSettings()
	if err != nil {
		t.Fatalf("GetServerMiscSettings after second upsert failed: %v", err)
	}
	if got2 == nil || got2.CBZConvertEnabled != false || len(got2.IgnoreDevices) != 0 {
		t.Errorf("GetServerMiscSettings after second upsert = %+v, want CBZConvertEnabled=false, empty IgnoreDevices", got2)
	}
}
