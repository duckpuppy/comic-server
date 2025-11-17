package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestUpdateLibraryReadingState_ReadingProgress tests updating reading progress fields
func TestUpdateLibraryReadingState_ReadingProgress(t *testing.T) {
	// Create temporary library file
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	// Create library with a book
	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:           "book1",
				Title:        "Test Book 1",
				FilePath:     "/test/book1.cbz",
				CurrentPage:  5,
				LastPageRead: 4,
				OpenCount:    2,
				OpenedTime:   library.ComicTime{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		},
	}

	// Save library to disk
	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Create device books with updated reading progress
	openedTime := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:           "book1",
				Title:        "Test Book 1",
				FilePath:     "/test/book1.cbz",
				CurrentPage:  10,
				LastPageRead: 9,
				OpenCount:    5,
				OpenedTime:   library.ComicTime{Time: openedTime},
			},
		},
	}

	// Create syncer and set library path
	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	// Update library from device
	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload library from disk to verify changes were saved
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	// Verify reading progress was updated
	book := reloadedLib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found in reloaded library")
	}

	if book.CurrentPage != 10 {
		t.Errorf("expected CurrentPage=10, got %d", book.CurrentPage)
	}
	if book.LastPageRead != 9 {
		t.Errorf("expected LastPageRead=9, got %d", book.LastPageRead)
	}
	if book.OpenCount != 5 {
		t.Errorf("expected OpenCount=5, got %d", book.OpenCount)
	}
	if !book.OpenedTime.Equal(openedTime) {
		t.Errorf("expected OpenedTime=%v, got %v", openedTime, book.OpenedTime.Time)
	}
}

// TestUpdateLibraryReadingState_UserMetadata tests updating user metadata fields
func TestUpdateLibraryReadingState_UserMetadata(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	// Create library with a book
	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:       "book1",
				Title:    "Test Book 1",
				FilePath: "/test/book1.cbz",
				Rating:   3.0,
				Notes:    "Old notes",
				Review:   "Old review",
				Summary:  "Old summary",
				Tags:     "old,tags",
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Create device books with updated user metadata
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:       "book1",
				Title:    "Test Book 1",
				FilePath: "/test/book1.cbz",
				Rating:   5.0,
				Notes:    "New notes from device",
				Review:   "New review from device",
				Summary:  "New summary from device",
				Tags:     "new,device,tags",
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload and verify
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	book := reloadedLib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found")
	}

	if book.Rating != 5.0 {
		t.Errorf("expected Rating=5.0, got %f", book.Rating)
	}
	if book.Notes != "New notes from device" {
		t.Errorf("expected Notes updated, got %s", book.Notes)
	}
	if book.Review != "New review from device" {
		t.Errorf("expected Review updated, got %s", book.Review)
	}
	if book.Summary != "New summary from device" {
		t.Errorf("expected Summary updated, got %s", book.Summary)
	}
	if book.Tags != "new,device,tags" {
		t.Errorf("expected Tags updated, got %s", book.Tags)
	}
}

// TestUpdateLibraryReadingState_CheckedFlag tests updating the Checked flag
func TestUpdateLibraryReadingState_CheckedFlag(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:       "book1",
				Title:    "Unchecked Book",
				FilePath: "/test/book1.cbz",
				Checked:  false,
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Device marks book as checked
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:       "book1",
				Title:    "Unchecked Book",
				FilePath: "/test/book1.cbz",
				Checked:  true,
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload and verify
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	book := reloadedLib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found")
	}

	if !book.Checked {
		t.Error("expected Checked=true, got false")
	}
}

// TestUpdateLibraryReadingState_PageMetadata tests updating page metadata
func TestUpdateLibraryReadingState_PageMetadata(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:       "book1",
				Title:    "Test Book",
				FilePath: "/test/book1.cbz",
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeStory},
				},
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Device has updated page metadata (bookmarks, page types, etc.)
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:       "book1",
				Title:    "Test Book",
				FilePath: "/test/book1.cbz",
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover, Bookmark: "Cover Page"},
					{Image: 1, Type: library.PageTypeStory, Bookmark: "Important Scene"},
					{Image: 2, Type: library.PageTypeBackCover},
				},
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload and verify
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	book := reloadedLib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found")
	}

	if len(book.Pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(book.Pages))
	}

	// Verify page 0 (cover with bookmark)
	if book.Pages[0].Type != library.PageTypeFrontCover {
		t.Errorf("page 0: expected type FrontCover, got %s", book.Pages[0].Type)
	}
	if book.Pages[0].Bookmark != "Cover Page" {
		t.Errorf("page 0: expected bookmark 'Cover Page', got %s", book.Pages[0].Bookmark)
	}

	// Verify page 1 (story with bookmark)
	if book.Pages[1].Type != library.PageTypeStory {
		t.Errorf("page 1: expected type Story, got %s", book.Pages[1].Type)
	}
	if book.Pages[1].Bookmark != "Important Scene" {
		t.Errorf("page 1: expected bookmark 'Important Scene', got %s", book.Pages[1].Bookmark)
	}

	// Verify page 2 (back cover added)
	if book.Pages[2].Type != library.PageTypeBackCover {
		t.Errorf("page 2: expected type BackCover, got %s", book.Pages[2].Type)
	}
}

// TestUpdateLibraryReadingState_DeviceBookNotInLibrary tests handling of device books not in library
func TestUpdateLibraryReadingState_DeviceBookNotInLibrary(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:       "book1",
				Title:    "Book 1",
				FilePath: "/test/book1.cbz",
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Device has a book that's not in library (maybe deleted from library)
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:          "book1",
				Title:       "Book 1",
				FilePath:    "/test/book1.cbz",
				CurrentPage: 10,
			},
		},
		"book2": {
			Filename:        "book2.cbp",
			SidecarFilename: "book2.cbp.xml",
			Metadata: &library.ComicBook{
				ID:          "book2",
				Title:       "Deleted Book",
				FilePath:    "/test/book2.cbz",
				CurrentPage: 5,
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	// Should not error, just skip book2
	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify library was still saved (book1 was updated)
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	// book1 should be updated
	book1 := reloadedLib.GetBook("book1")
	if book1 == nil {
		t.Fatal("book1 not found")
	}
	if book1.CurrentPage != 10 {
		t.Errorf("expected book1 CurrentPage=10, got %d", book1.CurrentPage)
	}

	// book2 should not be added to library
	book2 := reloadedLib.GetBook("book2")
	if book2 != nil {
		t.Error("book2 should not be in library")
	}
}

// TestUpdateLibraryReadingState_NoChanges tests that library is not saved when no changes
func TestUpdateLibraryReadingState_NoChanges(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:          "book1",
				Title:       "Test Book",
				FilePath:    "/test/book1.cbz",
				CurrentPage: 5,
				Rating:      4.0,
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Get original file modification time
	originalStat, err := os.Stat(libraryPath)
	if err != nil {
		t.Fatalf("failed to stat library: %v", err)
	}
	originalModTime := originalStat.ModTime()

	// Wait a bit to ensure modification time would change if file is written
	time.Sleep(10 * time.Millisecond)

	// Device has same metadata (no changes)
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:          "book1",
				Title:       "Test Book",
				FilePath:    "/test/book1.cbz",
				CurrentPage: 5,
				Rating:      4.0,
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	err = syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify library file was NOT modified
	newStat, err := os.Stat(libraryPath)
	if err != nil {
		t.Fatalf("failed to stat library: %v", err)
	}
	newModTime := newStat.ModTime()

	if !originalModTime.Equal(newModTime) {
		t.Errorf("library file was modified when no changes detected (original: %v, new: %v)",
			originalModTime, newModTime)
	}
}

// TestUpdateLibraryReadingState_NoMetadata tests handling of device books without metadata
func TestUpdateLibraryReadingState_NoMetadata(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:          "book1",
				Title:       "Test Book",
				FilePath:    "/test/book1.cbz",
				CurrentPage: 5,
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Device book has no metadata (sidecar failed to read)
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata:        nil, // No metadata
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	// Should not error, just skip the book
	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify library was not modified
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	book := reloadedLib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found")
	}

	// Should still have original CurrentPage
	if book.CurrentPage != 5 {
		t.Errorf("expected CurrentPage unchanged at 5, got %d", book.CurrentPage)
	}
}

// TestUpdateLibraryReadingState_NoLibraryPath tests handling when library path is not set
func TestUpdateLibraryReadingState_NoLibraryPath(t *testing.T) {
	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:          "book1",
				Title:       "Test Book",
				CurrentPage: 5,
			},
		},
	}

	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:          "book1",
				Title:       "Test Book",
				CurrentPage: 10,
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	// Don't set library path

	// Should not error, just skip the update
	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Library should NOT be updated when path is not set
	// (changes would be lost since we can't save them)
	book := lib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found")
	}

	if book.CurrentPage != 5 {
		t.Errorf("expected CurrentPage to remain unchanged at 5, got %d", book.CurrentPage)
	}
}

// TestUpdateLibraryReadingState_AllFieldsUpdated tests updating all supported fields
func TestUpdateLibraryReadingState_AllFieldsUpdated(t *testing.T) {
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "ComicDb.xml")

	openedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	lib := &library.ComicLibrary{
		ID: "test-library",
		Books: []library.ComicBook{
			{
				ID:           "book1",
				Title:        "Test Book",
				FilePath:     "/test/book1.cbz",
				CurrentPage:  0,
				LastPageRead: 0,
				OpenCount:    0,
				OpenedTime:   library.ComicTime{Time: openedTime},
				Rating:       0,
				Notes:        "",
				Review:       "",
				Summary:      "",
				Tags:         "",
				Checked:      false,
				Pages:        []library.ComicPageInfo{},
			},
		},
	}

	if err := library.SaveLibrary(libraryPath, lib); err != nil {
		t.Fatalf("failed to save library: %v", err)
	}

	// Device has ALL fields updated
	newOpenedTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	deviceBooks := map[string]*DeviceBook{
		"book1": {
			Filename:        "book1.cbp",
			SidecarFilename: "book1.cbp.xml",
			Metadata: &library.ComicBook{
				ID:           "book1",
				Title:        "Test Book",
				FilePath:     "/test/book1.cbz",
				CurrentPage:  42,
				LastPageRead: 41,
				OpenCount:    10,
				OpenedTime:   library.ComicTime{Time: newOpenedTime},
				Rating:       4.5,
				Notes:        "Great story!",
				Review:       "Loved it!",
				Summary:      "An epic tale",
				Tags:         "favorites,superhero,read",
				Checked:      true,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover, Bookmark: "Start"},
					{Image: 25, Type: library.PageTypeStory, Bookmark: "Climax"},
				},
			},
		},
	}

	mockClient := NewMockClient()
	syncer := NewSyncer(mockClient, lib)
	syncer.SetLibraryPath(libraryPath)

	err := syncer.updateLibraryReadingState(deviceBooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload and verify ALL fields
	reloadedLib, err := library.LoadLibrary(libraryPath)
	if err != nil {
		t.Fatalf("failed to reload library: %v", err)
	}

	book := reloadedLib.GetBook("book1")
	if book == nil {
		t.Fatal("book1 not found")
	}

	// Verify all fields
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"CurrentPage", book.CurrentPage, 42},
		{"LastPageRead", book.LastPageRead, 41},
		{"OpenCount", book.OpenCount, 10},
		{"Rating", book.Rating, 4.5},
		{"Notes", book.Notes, "Great story!"},
		{"Review", book.Review, "Loved it!"},
		{"Summary", book.Summary, "An epic tale"},
		{"Tags", book.Tags, "favorites,superhero,read"},
		{"Checked", book.Checked, true},
		{"Pages.Length", len(book.Pages), 2},
	}

	for _, tt := range tests {
		if tt.got != tt.expected {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, tt.got)
		}
	}

	// Verify OpenedTime separately (time comparison)
	if !book.OpenedTime.Equal(newOpenedTime) {
		t.Errorf("OpenedTime: expected %v, got %v", newOpenedTime, book.OpenedTime.Time)
	}

	// Verify page metadata
	if book.Pages[0].Type != library.PageTypeFrontCover {
		t.Errorf("Page 0 Type: expected FrontCover, got %s", book.Pages[0].Type)
	}
	if book.Pages[0].Bookmark != "Start" {
		t.Errorf("Page 0 Bookmark: expected 'Start', got %s", book.Pages[0].Bookmark)
	}
	if book.Pages[1].Image != 25 {
		t.Errorf("Page 1 Image: expected 25, got %d", book.Pages[1].Image)
	}
	if book.Pages[1].Bookmark != "Climax" {
		t.Errorf("Page 1 Bookmark: expected 'Climax', got %s", book.Pages[1].Bookmark)
	}
}

// TestHasPagesMetadataChanged tests the page metadata comparison function
func TestHasPagesMetadataChanged(t *testing.T) {
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
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeStory},
				},
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
					{Image: 1, Type: library.PageTypeStory},
				},
			},
			expected: false,
		},
		{
			name: "different page count",
			library: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0},
				},
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0},
					{Image: 1},
				},
			},
			expected: true,
		},
		{
			name: "different page type",
			library: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeFrontCover},
				},
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeBackCover},
				},
			},
			expected: true,
		},
		{
			name: "different bookmark",
			library: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0, Bookmark: "Old Bookmark"},
				},
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0, Bookmark: "New Bookmark"},
				},
			},
			expected: true,
		},
		{
			name: "different image index",
			library: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 0},
				},
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{
					{Image: 1},
				},
			},
			expected: true,
		},
		{
			name: "empty pages arrays",
			library: &library.ComicBook{
				Pages: []library.ComicPageInfo{},
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{},
			},
			expected: false,
		},
		{
			name: "nil vs empty pages",
			library: &library.ComicBook{
				Pages: nil,
			},
			device: &library.ComicBook{
				Pages: []library.ComicPageInfo{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncer.hasPagesMetadataChanged(tt.library, tt.device)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
