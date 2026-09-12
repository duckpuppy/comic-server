package configdb

import "testing"

func TestTrashSettings_GetReturnsNilWhenUnset(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetTrashSettings()
	if err != nil {
		t.Fatalf("GetTrashSettings failed: %v", err)
	}
	if got != nil {
		t.Errorf("GetTrashSettings on empty db = %+v, want nil", got)
	}
}

func TestTrashSettings_UpsertAndGet(t *testing.T) {
	db := newTestDB(t)

	s := TrashSettings{Path: "/data/trash", RetentionDays: 14}
	if err := db.UpsertTrashSettings(s); err != nil {
		t.Fatalf("UpsertTrashSettings failed: %v", err)
	}

	got, err := db.GetTrashSettings()
	if err != nil {
		t.Fatalf("GetTrashSettings failed: %v", err)
	}
	if got == nil || *got != s {
		t.Errorf("GetTrashSettings = %+v, want %+v", got, s)
	}

	// Upsert again overwrites wholesale, not merges.
	s2 := TrashSettings{Path: "/other/trash", RetentionDays: 7}
	if err := db.UpsertTrashSettings(s2); err != nil {
		t.Fatalf("second UpsertTrashSettings failed: %v", err)
	}
	got2, err := db.GetTrashSettings()
	if err != nil {
		t.Fatalf("GetTrashSettings after second upsert failed: %v", err)
	}
	if got2 == nil || *got2 != s2 {
		t.Errorf("GetTrashSettings after second upsert = %+v, want %+v", got2, s2)
	}
}
