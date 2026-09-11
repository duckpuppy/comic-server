package configdb

import "testing"

func TestGetTheme_DefaultsToSystemWhenUnset(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetTheme()
	if err != nil {
		t.Fatalf("GetTheme failed: %v", err)
	}
	if got != "system" {
		t.Errorf("GetTheme on empty db = %q, want %q", got, "system")
	}
}

func TestSetAndGetTheme(t *testing.T) {
	db := newTestDB(t)

	if err := db.SetTheme("dark"); err != nil {
		t.Fatalf("SetTheme failed: %v", err)
	}
	got, err := db.GetTheme()
	if err != nil {
		t.Fatalf("GetTheme failed: %v", err)
	}
	if got != "dark" {
		t.Errorf("GetTheme = %q, want %q", got, "dark")
	}

	// Overwriting must replace, not duplicate, the stored value.
	if err := db.SetTheme("light"); err != nil {
		t.Fatalf("SetTheme failed: %v", err)
	}
	got, err = db.GetTheme()
	if err != nil {
		t.Fatalf("GetTheme failed: %v", err)
	}
	if got != "light" {
		t.Errorf("GetTheme after overwrite = %q, want %q", got, "light")
	}
}

func TestSetTheme_RejectsInvalidValue(t *testing.T) {
	db := newTestDB(t)

	if err := db.SetTheme("blue"); err == nil {
		t.Error("expected SetTheme to reject an invalid theme, got nil error")
	}
}
