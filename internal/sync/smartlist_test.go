package sync

import (
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestSetFilterList tests setting a smart list filter
func TestSetFilterList(t *testing.T) {
	lib := &library.ComicLibrary{}
	syncer := NewSyncer(nil, lib)

	tests := []struct {
		name    string
		list    *library.ComicListItem
		wantErr bool
	}{
		{
			name: "valid smart list",
			list: &library.ComicListItem{
				Type: "ComicSmartListItem",
				Name: "Test List",
			},
			wantErr: false,
		},
		{
			name:    "nil list (clears filter)",
			list:    nil,
			wantErr: false,
		},
		{
			name: "invalid - reading list",
			list: &library.ComicListItem{
				Type: "ComicReadingList",
				Name: "Not Smart",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := syncer.SetFilterList(tt.list)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetFilterList() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestComputeSyncPlanWithFilter tests that smart list filtering works during sync planning
func TestComputeSyncPlanWithFilter(t *testing.T) {
	// Create library with multiple books
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "1", Series: "Batman", Year: 2020},
			{ID: "2", Series: "Batman", Year: 2019},
			{ID: "3", Series: "Superman", Year: 2020},
			{ID: "4", Series: "Spider-Man", Year: 2020},
		},
		ComicLists: []library.ComicListItem{
			{
				Type:        "ComicSmartListItem",
				Name:        "Recent Batman",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
					{Type: "Year", MatchOperator: "1", MatchValue: "2019"}, // Greater than 2019
				},
			},
		},
	}

	mockClient := &MockClient{}
	syncer := NewSyncer(mockClient, lib)

	t.Run("no filter - all books planned", func(t *testing.T) {
		// No filter set
		deviceBooks := make(map[string]*DeviceBook)
		ops, err := syncer.ComputeSyncPlan(deviceBooks)
		if err != nil {
			t.Fatalf("ComputeSyncPlan() error = %v", err)
		}

		// Should plan to add all 4 books
		if len(ops) != 4 {
			t.Errorf("Expected 4 operations, got %d", len(ops))
		}

		// All should be Add operations
		for _, op := range ops {
			if op.Type != OperationAdd {
				t.Errorf("Expected OperationAdd, got %v", op.Type)
			}
		}
	})

	t.Run("with filter - only matching books planned", func(t *testing.T) {
		// Set filter to "Recent Batman" list
		err := syncer.SetFilterList(&lib.ComicLists[0])
		if err != nil {
			t.Fatalf("SetFilterList() error = %v", err)
		}

		deviceBooks := make(map[string]*DeviceBook)
		ops, err := syncer.ComputeSyncPlan(deviceBooks)
		if err != nil {
			t.Fatalf("ComputeSyncPlan() error = %v", err)
		}

		// Should only plan to add book 1 (Batman 2020)
		if len(ops) != 1 {
			t.Errorf("Expected 1 operation, got %d", len(ops))
		}

		if len(ops) > 0 {
			if ops[0].Book.ID != "1" {
				t.Errorf("Expected book ID 1, got %s", ops[0].Book.ID)
			}
			if ops[0].Type != OperationAdd {
				t.Errorf("Expected OperationAdd, got %v", ops[0].Type)
			}
		}
	})

	t.Run("filtered book on device but not in filter - should delete", func(t *testing.T) {
		// Set filter to "Recent Batman" (only book 1)
		err := syncer.SetFilterList(&lib.ComicLists[0])
		if err != nil {
			t.Fatalf("SetFilterList() error = %v", err)
		}

		// Device has book 3 (Superman), which is NOT in the filter
		deviceBooks := map[string]*DeviceBook{
			"3": {
				Filename:        "3.cbp",
				SidecarFilename: "3.cbp.xml",
				Metadata:        &lib.Books[2],
			},
		}

		ops, err := syncer.ComputeSyncPlan(deviceBooks)
		if err != nil {
			t.Fatalf("ComputeSyncPlan() error = %v", err)
		}

		// Should have 2 operations:
		// 1. Add book 1 (Batman 2020) - in filter, not on device
		// 2. Delete book 3 (Superman) - on device, not in filter
		if len(ops) != 2 {
			t.Errorf("Expected 2 operations, got %d", len(ops))
		}

		// Check we have one add and one delete
		addCount := 0
		deleteCount := 0
		for _, op := range ops {
			switch op.Type {
			case OperationAdd:
				addCount++
				if op.Book.ID != "1" {
					t.Errorf("Expected to add book 1, got %s", op.Book.ID)
				}
			case OperationDelete:
				deleteCount++
				if op.Device.Filename != "3.cbp" {
					t.Errorf("Expected to delete book 3, got %s", op.Device.Filename)
				}
			}
		}

		if addCount != 1 {
			t.Errorf("Expected 1 add operation, got %d", addCount)
		}
		if deleteCount != 1 {
			t.Errorf("Expected 1 delete operation, got %d", deleteCount)
		}
	})
}

// TestComputeSyncPlanWithSettings tests that SharedListSettings are applied during sync planning
func TestComputeSyncPlanWithSettings(t *testing.T) {
	// Create library with multiple books
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "1", Series: "Batman", Volume: 1, Number: "1", Year: 2020, CurrentPage: 0, PageCount: 20},
			{ID: "2", Series: "Batman", Volume: 1, Number: "2", Year: 2020, CurrentPage: 0, PageCount: 20},
			{ID: "3", Series: "Superman", Volume: 1, Number: "1", Year: 2020, CurrentPage: 0, PageCount: 20},
			{ID: "4", Series: "Superman", Volume: 1, Number: "2", Year: 2020, CurrentPage: 20, PageCount: 20}, // Read
			{ID: "5", Series: "Wonder Woman", Volume: 1, Number: "1", Year: 2020, CurrentPage: 0, PageCount: 20},
		},
	}

	mockClient := &MockClient{}
	syncer := NewSyncer(mockClient, lib)

	t.Run("limit books by count", func(t *testing.T) {
		// Set settings to limit to 2 books, sorted by series
		settings := &SharedListSettings{
			Limit:        true,
			LimitValue:   2,
			Sort:         true,
			ListSortType: SortTypeSeries,
		}
		syncer.SetSettings(settings)

		deviceBooks := make(map[string]*DeviceBook)
		ops, err := syncer.ComputeSyncPlan(deviceBooks)
		if err != nil {
			t.Fatalf("ComputeSyncPlan() error = %v", err)
		}

		// Should only plan to add 2 books (Batman #1 and Batman #2 due to sorting + limit)
		if len(ops) != 2 {
			t.Errorf("Expected 2 operations, got %d", len(ops))
		}

		// Verify the books are Batman #1 and #2
		if len(ops) >= 2 {
			if ops[0].Book.Series != "Batman" || ops[1].Book.Series != "Batman" {
				t.Errorf("Expected Batman series, got %s and %s", ops[0].Book.Series, ops[1].Book.Series)
			}
		}
	})

	t.Run("only unread books", func(t *testing.T) {
		// Set settings to only sync unread books
		settings := &SharedListSettings{
			OnlyUnread:   true,
			Sort:         true,
			ListSortType: SortTypeSeries,
		}
		syncer.SetSettings(settings)

		deviceBooks := make(map[string]*DeviceBook)
		ops, err := syncer.ComputeSyncPlan(deviceBooks)
		if err != nil {
			t.Fatalf("ComputeSyncPlan() error = %v", err)
		}

		// Should plan to add 4 books (all except book 4 which is read)
		if len(ops) != 4 {
			t.Errorf("Expected 4 operations, got %d", len(ops))
		}

		// Verify book 4 is not in the list
		for _, op := range ops {
			if op.Book.ID == "4" {
				t.Errorf("Book 4 (read) should not be synced with OnlyUnread=true")
			}
		}
	})

	t.Run("combine filter list and settings", func(t *testing.T) {
		// Add a smart list for Batman only
		lib.ComicLists = []library.ComicListItem{
			{
				Type:        "ComicSmartListItem",
				Name:        "Batman Only",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
				},
			},
		}

		// Set filter to Batman only
		err := syncer.SetFilterList(&lib.ComicLists[0])
		if err != nil {
			t.Fatalf("SetFilterList() error = %v", err)
		}

		// Set settings to limit to 1 book
		settings := &SharedListSettings{
			Limit:        true,
			LimitValue:   1,
			Sort:         true,
			ListSortType: SortTypeSeries,
		}
		syncer.SetSettings(settings)

		deviceBooks := make(map[string]*DeviceBook)
		ops, err := syncer.ComputeSyncPlan(deviceBooks)
		if err != nil {
			t.Fatalf("ComputeSyncPlan() error = %v", err)
		}

		// Should only plan to add 1 Batman book
		if len(ops) != 1 {
			t.Errorf("Expected 1 operation, got %d", len(ops))
		}

		if len(ops) > 0 {
			if ops[0].Book.Series != "Batman" {
				t.Errorf("Expected Batman, got %s", ops[0].Book.Series)
			}
			if ops[0].Book.Number != "1" {
				t.Errorf("Expected Batman #1, got #%s", ops[0].Book.Number)
			}
		}
	})
}
