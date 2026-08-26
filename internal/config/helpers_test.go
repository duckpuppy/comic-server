package config

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

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
