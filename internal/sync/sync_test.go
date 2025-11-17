package sync

import (
	"encoding/xml"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestGetDeviceBooks tests retrieving and parsing books from device
func TestGetDeviceBooks(t *testing.T) {
	tests := []struct {
		name          string
		deviceFiles   map[string][]byte
		listResult    string
		expectedBooks int
		expectError   bool
	}{
		{
			name:          "empty device",
			deviceFiles:   map[string][]byte{},
			listResult:    "",
			expectedBooks: 0,
			expectError:   false,
		},
		{
			name: "single book with sidecar",
			deviceFiles: map[string][]byte{
				"book1.cbp":     []byte("comic data"),
				"book1.cbp.xml": []byte(`<?xml version="1.0"?><Book><Title>Test Book</Title><ID>book1</ID></Book>`),
			},
			listResult:    "book1.cbp\nbook1.cbp.xml",
			expectedBooks: 1,
			expectError:   false,
		},
		{
			name: "multiple books with sidecars",
			deviceFiles: map[string][]byte{
				"book1.cbp":     []byte("comic data 1"),
				"book1.cbp.xml": []byte(`<?xml version="1.0"?><Book><Title>Book 1</Title><ID>book1</ID></Book>`),
				"book2.cbp":     []byte("comic data 2"),
				"book2.cbp.xml": []byte(`<?xml version="1.0"?><Book><Title>Book 2</Title><ID>book2</ID></Book>`),
			},
			listResult:    "book1.cbp\nbook1.cbp.xml\nbook2.cbp\nbook2.cbp.xml",
			expectedBooks: 2,
			expectError:   false,
		},
		{
			name: "book without sidecar",
			deviceFiles: map[string][]byte{
				"book1.cbp": []byte("comic data"),
			},
			listResult:    "book1.cbp",
			expectedBooks: 1,
			expectError:   false,
		},
		{
			name: "invalid sidecar xml",
			deviceFiles: map[string][]byte{
				"book1.cbp":     []byte("comic data"),
				"book1.cbp.xml": []byte("invalid xml"),
			},
			listResult:    "book1.cbp\nbook1.cbp.xml",
			expectedBooks: 1, // Book still counted, just no metadata
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock client
			mockClient := NewMockClient()
			mockClient.ListFilesResult = tt.listResult
			for filename, data := range tt.deviceFiles {
				mockClient.AddFile(filename, data)
			}

			// Create syncer
			lib := &library.ComicLibrary{}
			syncer := NewSyncer(mockClient, lib)

			// Execute
			books, err := syncer.GetDeviceBooks()

			// Verify
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(books) != tt.expectedBooks {
				t.Errorf("expected %d books, got %d", tt.expectedBooks, len(books))
			}

			// Verify book metadata was parsed when available
			if tt.name == "single book with sidecar" {
				book, ok := books["book1"]
				if !ok {
					t.Fatal("expected book1 to be in results")
				}
				if book.Metadata == nil {
					t.Error("expected metadata to be parsed")
				} else if book.Metadata.Title != "Test Book" {
					t.Errorf("expected title 'Test Book', got '%s'", book.Metadata.Title)
				}
			}
		})
	}
}

// TestComputeSyncPlan tests the sync plan computation logic
func TestComputeSyncPlan(t *testing.T) {
	tests := []struct {
		name            string
		libraryBooks    []library.ComicBook
		deviceBooks     map[string]*DeviceBook
		expectedAdds    int
		expectedUpdates int
		expectedDeletes int
	}{
		{
			name:            "empty library and device",
			libraryBooks:    []library.ComicBook{},
			deviceBooks:     map[string]*DeviceBook{},
			expectedAdds:    0,
			expectedUpdates: 0,
			expectedDeletes: 0,
		},
		{
			name: "new books in library",
			libraryBooks: []library.ComicBook{
				{ID: "book1", Title: "Book 1"},
				{ID: "book2", Title: "Book 2"},
			},
			deviceBooks:     map[string]*DeviceBook{},
			expectedAdds:    2,
			expectedUpdates: 0,
			expectedDeletes: 0,
		},
		{
			name:         "books only on device",
			libraryBooks: []library.ComicBook{},
			deviceBooks: map[string]*DeviceBook{
				"book1": {Filename: "book1.cbp"},
				"book2": {Filename: "book2.cbp"},
			},
			expectedAdds:    0,
			expectedUpdates: 0,
			expectedDeletes: 2,
		},
		{
			name: "books unchanged",
			libraryBooks: []library.ComicBook{
				{ID: "book1", Title: "Book 1", PageCount: 10},
			},
			deviceBooks: map[string]*DeviceBook{
				"book1": {
					Filename: "book1.cbp",
					Metadata: &library.ComicBook{
						ID:        "book1",
						Title:     "Book 1",
						PageCount: 10,
					},
				},
			},
			expectedAdds:    0,
			expectedUpdates: 0,
			expectedDeletes: 0,
		},
		{
			name: "metadata changed",
			libraryBooks: []library.ComicBook{
				{ID: "book1", Title: "Updated Title", PageCount: 10},
			},
			deviceBooks: map[string]*DeviceBook{
				"book1": {
					Filename: "book1.cbp",
					Metadata: &library.ComicBook{
						ID:        "book1",
						Title:     "Old Title",
						PageCount: 10,
					},
				},
			},
			expectedAdds:    0,
			expectedUpdates: 1, // UpdateMetadataOnly
			expectedDeletes: 0,
		},
		{
			name: "pages changed",
			libraryBooks: []library.ComicBook{
				{ID: "book1", Title: "Book 1", PageCount: 15},
			},
			deviceBooks: map[string]*DeviceBook{
				"book1": {
					Filename: "book1.cbp",
					Metadata: &library.ComicBook{
						ID:        "book1",
						Title:     "Book 1",
						PageCount: 10,
					},
				},
			},
			expectedAdds:    0,
			expectedUpdates: 1, // Full Update
			expectedDeletes: 0,
		},
		{
			name: "mixed operations",
			libraryBooks: []library.ComicBook{
				{ID: "book1", Title: "Unchanged", PageCount: 10},
				{ID: "book2", Title: "New Book"},
				{ID: "book3", Title: "Updated Metadata", PageCount: 10},
			},
			deviceBooks: map[string]*DeviceBook{
				"book1": {
					Filename: "book1.cbp",
					Metadata: &library.ComicBook{
						ID:        "book1",
						Title:     "Unchanged",
						PageCount: 10,
					},
				},
				"book3": {
					Filename: "book3.cbp",
					Metadata: &library.ComicBook{
						ID:        "book3",
						Title:     "Old Metadata",
						PageCount: 10,
					},
				},
				"book4": {
					Filename: "book4.cbp",
					Metadata: &library.ComicBook{
						ID:    "book4",
						Title: "To Be Deleted",
					},
				},
			},
			expectedAdds:    1, // book2
			expectedUpdates: 1, // book3
			expectedDeletes: 1, // book4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			lib := &library.ComicLibrary{Books: tt.libraryBooks}
			mockClient := NewMockClient()
			syncer := NewSyncer(mockClient, lib)

			// Execute
			operations, err := syncer.ComputeSyncPlan(tt.deviceBooks)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Count operations by type
			adds := 0
			updates := 0
			deletes := 0
			for _, op := range operations {
				switch op.Type {
				case OperationAdd:
					adds++
				case OperationUpdate, OperationUpdateMetadataOnly:
					updates++
				case OperationDelete:
					deletes++
				}
			}

			// Verify
			if adds != tt.expectedAdds {
				t.Errorf("expected %d adds, got %d", tt.expectedAdds, adds)
			}
			if updates != tt.expectedUpdates {
				t.Errorf("expected %d updates, got %d", tt.expectedUpdates, updates)
			}
			if deletes != tt.expectedDeletes {
				t.Errorf("expected %d deletes, got %d", tt.expectedDeletes, deletes)
			}
		})
	}
}

// TestCompareBooks tests book comparison logic
func TestCompareBooks(t *testing.T) {
	syncer := &Syncer{}

	tests := []struct {
		name           string
		libraryBook    *library.ComicBook
		deviceMetadata *library.ComicBook
		expectedType   OperationType
		expectUpdate   bool
	}{
		{
			name: "no device metadata",
			libraryBook: &library.ComicBook{
				ID:    "book1",
				Title: "Test",
			},
			deviceMetadata: nil,
			expectedType:   OperationUpdate,
			expectUpdate:   true,
		},
		{
			name: "identical books",
			libraryBook: &library.ComicBook{
				ID:        "book1",
				Title:     "Test Book",
				Series:    "Test Series",
				Number:    "1",
				PageCount: 10,
			},
			deviceMetadata: &library.ComicBook{
				ID:        "book1",
				Title:     "Test Book",
				Series:    "Test Series",
				Number:    "1",
				PageCount: 10,
			},
			expectedType: OperationType(0),
			expectUpdate: false,
		},
		{
			name: "title changed",
			libraryBook: &library.ComicBook{
				ID:        "book1",
				Title:     "New Title",
				PageCount: 10,
			},
			deviceMetadata: &library.ComicBook{
				ID:        "book1",
				Title:     "Old Title",
				PageCount: 10,
			},
			expectedType: OperationUpdateMetadataOnly,
			expectUpdate: true,
		},
		{
			name: "page count changed",
			libraryBook: &library.ComicBook{
				ID:        "book1",
				Title:     "Test",
				PageCount: 15,
			},
			deviceMetadata: &library.ComicBook{
				ID:        "book1",
				Title:     "Test",
				PageCount: 10,
			},
			expectedType: OperationUpdate,
			expectUpdate: true,
		},
		{
			name: "pages array changed",
			libraryBook: &library.ComicBook{
				ID:        "book1",
				Title:     "Test",
				PageCount: 2,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeStory},
				},
			},
			deviceMetadata: &library.ComicBook{
				ID:        "book1",
				Title:     "Test",
				PageCount: 2,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeBackCover},
				},
			},
			expectedType: OperationUpdate,
			expectUpdate: true,
		},
		{
			name: "multiple metadata fields changed",
			libraryBook: &library.ComicBook{
				ID:        "book1",
				Title:     "New Title",
				Series:    "New Series",
				Number:    "2",
				PageCount: 10,
			},
			deviceMetadata: &library.ComicBook{
				ID:        "book1",
				Title:     "Old Title",
				Series:    "Old Series",
				Number:    "1",
				PageCount: 10,
			},
			expectedType: OperationUpdateMetadataOnly,
			expectUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceBook := &DeviceBook{
				Filename: "test.cbp",
				Metadata: tt.deviceMetadata,
			}

			op, needsUpdate := syncer.compareBooks(tt.libraryBook, deviceBook)

			if needsUpdate != tt.expectUpdate {
				t.Errorf("expected needsUpdate=%v, got %v", tt.expectUpdate, needsUpdate)
			}

			if needsUpdate && op.Type != tt.expectedType {
				t.Errorf("expected operation type %v, got %v", tt.expectedType, op.Type)
			}
		})
	}
}

// TestHasMetadataChanged tests metadata change detection
func TestHasMetadataChanged(t *testing.T) {
	syncer := &Syncer{}

	tests := []struct {
		name     string
		library  *library.ComicBook
		device   *library.ComicBook
		expected bool
	}{
		{
			name:     "identical books",
			library:  &library.ComicBook{Title: "Test", Series: "Series"},
			device:   &library.ComicBook{Title: "Test", Series: "Series"},
			expected: false,
		},
		{
			name:     "title changed",
			library:  &library.ComicBook{Title: "New"},
			device:   &library.ComicBook{Title: "Old"},
			expected: true,
		},
		{
			name:     "series changed",
			library:  &library.ComicBook{Series: "New"},
			device:   &library.ComicBook{Series: "Old"},
			expected: true,
		},
		{
			name:     "number changed",
			library:  &library.ComicBook{Number: "2"},
			device:   &library.ComicBook{Number: "1"},
			expected: true,
		},
		{
			name:     "volume changed",
			library:  &library.ComicBook{Volume: 2},
			device:   &library.ComicBook{Volume: 1},
			expected: true,
		},
		{
			name:     "writer changed",
			library:  &library.ComicBook{Writer: "New Writer"},
			device:   &library.ComicBook{Writer: "Old Writer"},
			expected: true,
		},
		{
			name:     "publisher changed",
			library:  &library.ComicBook{Publisher: "New Pub"},
			device:   &library.ComicBook{Publisher: "Old Pub"},
			expected: true,
		},
		{
			name:     "year changed",
			library:  &library.ComicBook{Year: 2024},
			device:   &library.ComicBook{Year: 2023},
			expected: true,
		},
		{
			name:     "rating changed",
			library:  &library.ComicBook{Rating: 5},
			device:   &library.ComicBook{Rating: 4},
			expected: true,
		},
		{
			name:     "current page changed",
			library:  &library.ComicBook{CurrentPage: 10},
			device:   &library.ComicBook{CurrentPage: 5},
			expected: true,
		},
		{
			name:     "summary changed",
			library:  &library.ComicBook{Summary: "New summary"},
			device:   &library.ComicBook{Summary: "Old summary"},
			expected: true,
		},
		{
			name:     "notes changed",
			library:  &library.ComicBook{Notes: "New notes"},
			device:   &library.ComicBook{Notes: "Old notes"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncer.hasMetadataChanged(tt.library, tt.device)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestHasPagesChanged tests page structure change detection
func TestHasPagesChanged(t *testing.T) {
	syncer := &Syncer{}

	tests := []struct {
		name     string
		library  *library.ComicBook
		device   *library.ComicBook
		expected bool
	}{
		{
			name: "identical pages",
			library: &library.ComicBook{
				PageCount: 2,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeStory},
				},
			},
			device: &library.ComicBook{
				PageCount: 2,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeStory},
				},
			},
			expected: false,
		},
		{
			name: "page count changed",
			library: &library.ComicBook{
				PageCount: 3,
			},
			device: &library.ComicBook{
				PageCount: 2,
			},
			expected: true,
		},
		{
			name: "page array length changed",
			library: &library.ComicBook{
				PageCount: 2,
				Pages: []library.ComicPageInfo{
					{Image: 0},
					{Image: 1},
				},
			},
			device: &library.ComicBook{
				PageCount: 2,
				Pages: []library.ComicPageInfo{
					{Image: 0},
				},
			},
			expected: true,
		},
		{
			name: "page type changed",
			library: &library.ComicBook{
				PageCount: 1,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeBackCover},
				},
			},
			device: &library.ComicBook{
				PageCount: 1,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
				},
			},
			expected: true,
		},
		{
			name: "page image index changed",
			library: &library.ComicBook{
				PageCount: 1,
				Pages: []library.ComicPageInfo{
					{Image: 1, Type: library.PageTypeStory},
				},
			},
			device: &library.ComicBook{
				PageCount: 1,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeStory},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncer.hasPagesChanged(tt.library, tt.device)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestOperationTypeString tests OperationType string representation
func TestOperationTypeString(t *testing.T) {
	tests := []struct {
		op       OperationType
		expected string
	}{
		{OperationAdd, "Add"},
		{OperationUpdate, "Update"},
		{OperationDelete, "Delete"},
		{OperationUpdateMetadataOnly, "UpdateMetadataOnly"},
		{OperationType(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.op.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestGenerateSidecar tests sidecar XML generation
func TestGenerateSidecar(t *testing.T) {
	mockClient := NewMockClient()
	lib := &library.ComicLibrary{}
	syncer := NewSyncer(mockClient, lib)

	book := &library.ComicBook{
		ID:        "test-book",
		Title:     "Test Book",
		Series:    "Test Series",
		Number:    "1",
		PageCount: 10,
	}

	data, err := syncer.generateSidecar(book)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid XML
	var parsed library.ComicBook
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated invalid XML: %v", err)
	}

	// Verify content
	if parsed.ID != book.ID {
		t.Errorf("expected ID %s, got %s", book.ID, parsed.ID)
	}
	if parsed.Title != book.Title {
		t.Errorf("expected Title %s, got %s", book.Title, parsed.Title)
	}
}

func TestSetFilterLists_MultipleSmartLists(t *testing.T) {
	// Create test library with books
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Series: "Series A", Title: "Book 1"},
			{ID: "book2", Series: "Series B", Title: "Book 2"},
			{ID: "book3", Series: "Series A", Title: "Book 3"},
			{ID: "book4", Series: "Series C", Title: "Book 4"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list1",
				Name:        "Series A Books",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series A"},
				},
			},
			{
				ID:          "list2",
				Name:        "Series B Books",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series B"},
				},
			},
			{
				ID:   "list3",
				Name: "Regular List",
				Type: "ReadingList",
			},
		},
	}

	client := NewMockClient()
	syncer := NewSyncer(client, lib)

	// Test setting multiple smart lists
	smartList1 := &lib.ComicLists[0]
	smartList2 := &lib.ComicLists[1]

	err := syncer.SetFilterLists([]*library.ComicListItem{smartList1, smartList2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify filterLists is set and filterList is cleared
	if len(syncer.filterLists) != 2 {
		t.Errorf("expected 2 filter lists, got %d", len(syncer.filterLists))
	}
	if syncer.filterList != nil {
		t.Error("expected filterList to be cleared when filterLists is set")
	}

	// Test setting non-smart list should fail
	regularList := &lib.ComicLists[2]
	err = syncer.SetFilterLists([]*library.ComicListItem{smartList1, regularList})
	if err == nil {
		t.Error("expected error when setting non-smart list, got nil")
	}
}

func TestSetFilterLists_BackwardCompatibility(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list1",
				Name:        "Smart List",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Title", MatchOperator: "2", MatchValue: "Book"}, // Contains "Book"
				},
			},
		},
	}

	client := NewMockClient()
	syncer := NewSyncer(client, lib)

	smartList := &lib.ComicLists[0]

	// Test that SetFilterList clears filterLists
	syncer.filterLists = []*library.ComicListItem{smartList}
	err := syncer.SetFilterList(smartList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if syncer.filterList != smartList {
		t.Error("expected filterList to be set")
	}
	if len(syncer.filterLists) != 0 {
		t.Error("expected filterLists to be cleared when filterList is set")
	}

	// Test that SetFilterLists clears filterList
	syncer.filterList = smartList
	err = syncer.SetFilterLists([]*library.ComicListItem{smartList})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(syncer.filterLists) != 1 {
		t.Error("expected filterLists to be set")
	}
	if syncer.filterList != nil {
		t.Error("expected filterList to be cleared when filterLists is set")
	}
}

func TestComputeUnionOfLists(t *testing.T) {
	// Create test library with overlapping books in lists
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Series: "Series A", Title: "Book 1"},
			{ID: "book2", Series: "Series B", Title: "Book 2"},
			{ID: "book3", Series: "Series A", Title: "Book 3"},
			{ID: "book4", Series: "Series C", Title: "Book 4"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list1",
				Name:        "List 1 (Series A or B)",
				Type:        "ComicSmartListItem",
				MatcherMode: "Or",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series A"},
					{Type: "Series", MatchOperator: "0", MatchValue: "Series B"},
				},
			},
			{
				ID:          "list2",
				Name:        "List 2 (Series B only)",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series B"}, // book2 overlaps with list1
				},
			},
		},
	}

	client := NewMockClient()
	syncer := NewSyncer(client, lib)

	// Set multiple filter lists
	err := syncer.SetFilterLists([]*library.ComicListItem{
		&lib.ComicLists[0],
		&lib.ComicLists[1],
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compute union
	books := syncer.computeUnionOfLists()

	// Verify union contains book1, book2, book3 (no duplicates)
	if len(books) != 3 {
		t.Errorf("expected 3 books in union, got %d", len(books))
	}

	// Verify book IDs
	bookIDs := make(map[string]bool)
	for _, book := range books {
		bookIDs[book.ID] = true
	}

	expectedIDs := []string{"book1", "book2", "book3"}
	for _, id := range expectedIDs {
		if !bookIDs[id] {
			t.Errorf("expected book %s in union, but not found", id)
		}
	}

	// Verify book4 is NOT in union (not in any list)
	if bookIDs["book4"] {
		t.Error("book4 should not be in union")
	}
}

func TestComputeSyncPlan_MultipleFilterLists(t *testing.T) {
	// Create test library
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Series: "Series A", Title: "Book 1", FilePath: "/path/book1.cbz"},
			{ID: "book2", Series: "Series B", Title: "Book 2", FilePath: "/path/book2.cbz"},
			{ID: "book3", Series: "Series A", Title: "Book 3", FilePath: "/path/book3.cbz"},
			{ID: "book4", Series: "Series C", Title: "Book 4", FilePath: "/path/book4.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list1",
				Name:        "List 1 (Series A only)",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series A"},
				},
			},
			{
				ID:          "list2",
				Name:        "List 2 (Series B only)",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series B"},
				},
			},
		},
	}

	client := NewMockClient()
	syncer := NewSyncer(client, lib)

	// Set multiple filter lists
	err := syncer.SetFilterLists([]*library.ComicListItem{
		&lib.ComicLists[0], // book1, book3 (Series A)
		&lib.ComicLists[1], // book2 (Series B)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Device has no books
	deviceBooks := make(map[string]*DeviceBook)

	// Compute sync plan
	operations, err := syncer.ComputeSyncPlan(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 add operations (book1, book2, book3) from union of lists
	// book1 & book3 from list1 (Series A), book2 from list2 (Series B)
	// book4 should NOT be included (Series C not in any list)
	if len(operations) != 3 {
		t.Errorf("expected 3 operations, got %d", len(operations))
	}

	// Verify all are add operations
	for _, op := range operations {
		if op.Type != OperationAdd {
			t.Errorf("expected OperationAdd, got %v", op.Type)
		}
	}

	// Verify book IDs
	bookIDs := make(map[string]bool)
	for _, op := range operations {
		bookIDs[op.Book.ID] = true
	}

	expectedIDs := []string{"book1", "book2", "book3"}
	for _, id := range expectedIDs {
		if !bookIDs[id] {
			t.Errorf("expected book %s in operations, but not found", id)
		}
	}

	if bookIDs["book4"] {
		t.Error("book4 should not be in operations (not in any filter list)")
	}
}

func TestComputeSyncPlan_SingleFilterList_BackwardCompatibility(t *testing.T) {
	// Create test library
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: "/path/book1.cbz"},
			{ID: "book2", Title: "Book 2", FilePath: "/path/book2.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list1",
				Name:        "Smart List",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Title", MatchOperator: "2", MatchValue: "Book 1"}, // Contains "Book 1"
				},
			},
		},
	}

	client := NewMockClient()
	syncer := NewSyncer(client, lib)

	// Use deprecated SetFilterList (backward compatibility)
	err := syncer.SetFilterList(&lib.ComicLists[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deviceBooks := make(map[string]*DeviceBook)

	// Compute sync plan
	operations, err := syncer.ComputeSyncPlan(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 add operation (book1 only)
	if len(operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(operations))
	}

	if operations[0].Book.ID != "book1" {
		t.Errorf("expected book1, got %s", operations[0].Book.ID)
	}
}
