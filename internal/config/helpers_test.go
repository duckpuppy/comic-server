package config

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

func TestResolveDevice(t *testing.T) {
	cfg := NewConfig()
	cfg.AddDevice("device-123", "My Tablet")
	cfg.AddDevice("device-456", "My Phone")
	cfg.AddDevice("device-789", "Other Device")

	tests := []struct {
		name        string
		nameOrID    string
		wantID      string
		wantName    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "exact ID match",
			nameOrID: "device-123",
			wantID:   "device-123",
			wantName: "My Tablet",
			wantErr:  false,
		},
		{
			name:     "exact friendly name match",
			nameOrID: "My Tablet",
			wantID:   "device-123",
			wantName: "My Tablet",
			wantErr:  false,
		},
		{
			name:     "partial name match (unique)",
			nameOrID: "tablet",
			wantID:   "device-123",
			wantName: "My Tablet",
			wantErr:  false,
		},
		{
			name:     "partial name match (case insensitive)",
			nameOrID: "PHONE",
			wantID:   "device-456",
			wantName: "My Phone",
			wantErr:  false,
		},
		{
			name:        "not found",
			nameOrID:    "nonexistent",
			wantErr:     true,
			errContains: "device not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotDevice, err := ResolveDevice(cfg, tt.nameOrID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolveDevice() expected error containing %q, got nil", tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveDevice() unexpected error = %v", err)
			}

			if gotID != tt.wantID {
				t.Errorf("ResolveDevice() ID = %v, want %v", gotID, tt.wantID)
			}

			if gotDevice == nil {
				t.Fatal("ResolveDevice() returned nil device")
			}

			if gotDevice.FriendlyName != tt.wantName {
				t.Errorf("ResolveDevice() device name = %v, want %v", gotDevice.FriendlyName, tt.wantName)
			}
		})
	}
}

func TestResolveDeviceAmbiguous(t *testing.T) {
	cfg := NewConfig()
	cfg.AddDevice("device-1", "My Device")
	cfg.AddDevice("device-2", "My Device")

	_, _, err := ResolveDevice(cfg, "My Device")
	if err == nil {
		t.Error("ResolveDevice() should error for ambiguous name")
	}
}

func TestResolveSmartList(t *testing.T) {
	// Create a test library
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{
				Type: "comicrack:ComicSmartListItem",
				ID:   "list-guid-1",
				Name: "Currently Reading",
				Matchers: []library.ComicBookMatcher{
					{Type: "PageCount", MatchOperator: "GreaterThan", MatchValue: "0"},
				},
			},
			{
				Type: "comicrack:ComicSmartListItem",
				ID:   "list-guid-2",
				Name: "Favorites",
				Matchers: []library.ComicBookMatcher{
					{Type: "Rating", MatchOperator: "GreaterThan", MatchValue: "4"},
				},
			},
			{
				Type: "comicrack:ComicSmartListItem",
				ID:   "list-guid-3",
				Name: "Reading List",
				Matchers: []library.ComicBookMatcher{
					{Type: "Read", MatchOperator: "Equal", MatchValue: "false"},
				},
			},
		},
	}

	tests := []struct {
		name        string
		listName    string
		wantID      string
		wantName    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "exact name match",
			listName: "Favorites",
			wantID:   "list-guid-2",
			wantName: "Favorites",
			wantErr:  false,
		},
		{
			name:     "another exact name match",
			listName: "Currently Reading",
			wantID:   "list-guid-1",
			wantName: "Currently Reading",
			wantErr:  false,
		},
		{
			name:        "not found",
			listName:    "Nonexistent List",
			wantErr:     true,
			errContains: "smart list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotName, err := ResolveSmartList(lib, tt.listName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolveSmartList() expected error containing %q, got nil", tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveSmartList() unexpected error = %v", err)
			}

			if gotID != tt.wantID {
				t.Errorf("ResolveSmartList() ID = %v, want %v", gotID, tt.wantID)
			}

			if gotName != tt.wantName {
				t.Errorf("ResolveSmartList() name = %v, want %v", gotName, tt.wantName)
			}
		})
	}
}

func TestResolveSmartListAmbiguous(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{Type: "comicrack:ComicSmartListItem", ID: "list-1", Name: "Reading List"},
			{Type: "comicrack:ComicSmartListItem", ID: "list-2", Name: "Reading Queue"},
		},
	}

	_, _, err := ResolveSmartList(lib, "reading")
	if err == nil {
		t.Error("ResolveSmartList() should error for ambiguous name")
	}
}

func TestFindListByGUID(t *testing.T) {
	lib := &library.ComicLibrary{
		ComicLists: []library.ComicListItem{
			{Type: "comicrack:ComicSmartListItem", ID: "list-guid-1", Name: "List One"},
			{Type: "comicrack:ComicSmartListItem", ID: "list-guid-2", Name: "List Two"},
		},
	}

	t.Run("found", func(t *testing.T) {
		list := FindListByGUID(lib, "list-guid-1")
		if list == nil {
			t.Fatal("FindListByGUID() returned nil for existing list")
		}

		if list.Name != "List One" {
			t.Errorf("FindListByGUID() name = %v, want List One", list.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		list := FindListByGUID(lib, "nonexistent")
		if list != nil {
			t.Error("FindListByGUID() should return nil for nonexistent list")
		}
	})
}

func TestFormatSmartListConfig(t *testing.T) {
	listConfig := &SharedListConfig{
		ListID:   "test-guid",
		ListName: "Test List",
		Enabled:  true,
		Settings: nil,
	}

	output := FormatSmartListConfig(listConfig)

	if output == "" {
		t.Error("FormatSmartListConfig() returned empty string")
	}

	// Basic checks that output contains expected elements
	if !contains(output, "Test List") {
		t.Error("FormatSmartListConfig() output missing list name")
	}

	if !contains(output, "enabled") {
		t.Error("FormatSmartListConfig() output missing enabled indicator")
	}
}

func TestFormatSmartListConfigDisabled(t *testing.T) {
	listConfig := &SharedListConfig{
		ListID:   "test-guid",
		ListName: "Disabled List",
		Enabled:  false,
		Settings: nil,
	}

	output := FormatSmartListConfig(listConfig)

	if !contains(output, "disabled") {
		t.Error("FormatSmartListConfig() output missing disabled indicator")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
