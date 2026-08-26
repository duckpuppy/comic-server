package configdb

import (
	"database/sql"
	"errors"
	"testing"
)

func TestKomgaTarget_CreateGetListDeleteUpdate(t *testing.T) {
	db := newTestDB(t)

	if got, err := db.GetKomgaTarget("list-1"); err != nil || got != nil {
		t.Fatalf("GetKomgaTarget on empty db = %+v, %v; want nil, nil", got, err)
	}

	target := KomgaTarget{
		ListID:         "list-1",
		ListName:       "Currently Reading",
		Type:           "collection",
		KomgaName:      "Batman Collection",
		Enabled:        true,
		SyncReadStatus: false,
	}
	if err := db.CreateKomgaTarget(target); err != nil {
		t.Fatalf("CreateKomgaTarget failed: %v", err)
	}

	got, err := db.GetKomgaTarget("list-1")
	if err != nil {
		t.Fatalf("GetKomgaTarget failed: %v", err)
	}
	if got == nil || *got != target {
		t.Errorf("GetKomgaTarget = %+v, want %+v", got, target)
	}

	all, err := db.ListKomgaTargets()
	if err != nil {
		t.Fatalf("ListKomgaTargets failed: %v", err)
	}
	if len(all) != 1 || all[0] != target {
		t.Errorf("ListKomgaTargets = %+v, want [%+v]", all, target)
	}

	if err := db.UpdateKomgaTarget("list-1", "readlist", "Batman Read List", false, true); err != nil {
		t.Fatalf("UpdateKomgaTarget failed: %v", err)
	}
	got, err = db.GetKomgaTarget("list-1")
	if err != nil {
		t.Fatalf("GetKomgaTarget after update failed: %v", err)
	}
	want := KomgaTarget{
		ListID:         "list-1",
		ListName:       "Currently Reading", // list_name isn't touched by UpdateKomgaTarget
		Type:           "readlist",
		KomgaName:      "Batman Read List",
		Enabled:        false,
		SyncReadStatus: true,
	}
	if got == nil || *got != want {
		t.Errorf("GetKomgaTarget after update = %+v, want %+v", got, want)
	}

	if err := db.DeleteKomgaTarget("list-1"); err != nil {
		t.Fatalf("DeleteKomgaTarget failed: %v", err)
	}
	if got, err := db.GetKomgaTarget("list-1"); err != nil || got != nil {
		t.Errorf("GetKomgaTarget after delete = %+v, %v; want nil, nil", got, err)
	}
}

func TestKomgaTarget_CreateDuplicateRejected(t *testing.T) {
	db := newTestDB(t)

	target := KomgaTarget{ListID: "list-1", Type: "collection", KomgaName: "X", Enabled: true}
	if err := db.CreateKomgaTarget(target); err != nil {
		t.Fatalf("CreateKomgaTarget failed: %v", err)
	}
	if err := db.CreateKomgaTarget(target); err == nil {
		t.Error("expected CreateKomgaTarget to reject a duplicate list_id")
	}
}

func TestKomgaTarget_UpdateNonexistentReturnsErrNoRows(t *testing.T) {
	db := newTestDB(t)

	err := db.UpdateKomgaTarget("does-not-exist", "collection", "X", true, false)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("UpdateKomgaTarget on nonexistent target = %v, want sql.ErrNoRows", err)
	}
}

func TestKomgaTarget_DeleteNonexistentIsNotAnError(t *testing.T) {
	db := newTestDB(t)

	if err := db.DeleteKomgaTarget("does-not-exist"); err != nil {
		t.Errorf("DeleteKomgaTarget on nonexistent target = %v, want nil", err)
	}
}
