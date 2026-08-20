package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Helper function to create test books
func createTestBook(id, series string, year, volume int, number string, pageCount, currentPage int) *library.ComicBook {
	return &library.ComicBook{
		ID:          id,
		Series:      series,
		Year:        year,
		Month:       1,
		Day:         1,
		Volume:      volume,
		Number:      number,
		PageCount:   pageCount,
		CurrentPage: currentPage,
		Checked:     false,
	}
}

// TestFilterOnlyUnread tests filtering for unread books
func TestFilterOnlyUnread(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),  // Unread
		createTestBook("2", "Batman", 2020, 1, "2", 20, 10), // Partially read
		createTestBook("3", "Batman", 2020, 1, "3", 20, 20), // Fully read
		createTestBook("4", "Batman", 2020, 1, "4", 20, 5),  // Partially read
		createTestBook("5", "Batman", 2020, 1, "5", 0, 0),   // No page tracking
	}

	result := filterOnlyUnread(books)

	// Should include: unread (0), partially read (10, 5), and no tracking (0)
	expectedIDs := map[string]bool{"1": true, "2": true, "4": true, "5": true}
	if len(result) != len(expectedIDs) {
		t.Errorf("Expected %d unread books, got %d", len(expectedIDs), len(result))
	}

	for _, book := range result {
		if !expectedIDs[book.ID] {
			t.Errorf("Unexpected book in unread results: %s", book.ID)
		}
	}
}

// TestFilterOnlyChecked tests filtering for checked books
func TestFilterOnlyChecked(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),
		createTestBook("2", "Batman", 2020, 1, "2", 20, 0),
		createTestBook("3", "Batman", 2020, 1, "3", 20, 0),
	}
	books[0].Checked = true
	books[2].Checked = true

	result := filterOnlyChecked(books)

	if len(result) != 2 {
		t.Errorf("Expected 2 checked books, got %d", len(result))
	}

	expectedIDs := map[string]bool{"1": true, "3": true}
	for _, book := range result {
		if !expectedIDs[book.ID] {
			t.Errorf("Unexpected book in checked results: %s", book.ID)
		}
	}
}

// TestGetMostRecentlyOpened tests getting recently opened books
func TestGetMostRecentlyOpened(t *testing.T) {
	now := time.Now()
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 20),
		createTestBook("2", "Batman", 2020, 1, "2", 20, 20),
		createTestBook("3", "Batman", 2020, 1, "3", 20, 20),
		createTestBook("4", "Batman", 2020, 1, "4", 20, 20),
	}
	books[0].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -4)} // 4 days ago
	books[1].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -1)} // 1 day ago
	books[2].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -3)} // 3 days ago
	books[3].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -2)} // 2 days ago

	result := getMostRecentlyOpened(books, 2)

	if len(result) != 2 {
		t.Fatalf("Expected 2 books, got %d", len(result))
	}

	// Should get books 2 (1 day ago) and 4 (2 days ago)
	if result[0].ID != "2" {
		t.Errorf("Expected first book to be ID 2, got %s", result[0].ID)
	}
	if result[1].ID != "4" {
		t.Errorf("Expected second book to be ID 4, got %s", result[1].ID)
	}
}

// TestSortBySeries tests sorting by series/volume/number
func TestSortBySeries(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Superman", 2020, 1, "5", 20, 0),
		createTestBook("2", "Batman", 2020, 2, "1", 20, 0),
		createTestBook("3", "Batman", 2020, 1, "2", 20, 0),
		createTestBook("4", "Batman", 2020, 1, "1", 20, 0),
		createTestBook("5", "Aquaman", 2020, 1, "1", 20, 0),
	}

	sortBySeries(books)

	// Expected order: Aquaman, Batman v1#1, Batman v1#2, Batman v2#1, Superman
	expectedOrder := []string{"5", "4", "3", "2", "1"}
	for i, expectedID := range expectedOrder {
		if books[i].ID != expectedID {
			t.Errorf("Position %d: expected ID %s, got %s", i, expectedID, books[i].ID)
		}
	}
}

// TestSortByPublished tests sorting by publication date
func TestSortByPublished(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),
		createTestBook("2", "Batman", 2018, 1, "1", 20, 0),
		createTestBook("3", "Batman", 2020, 1, "2", 20, 0),
		createTestBook("4", "Batman", 2019, 1, "1", 20, 0),
	}
	books[0].Month = 6
	books[1].Month = 1
	books[2].Month = 3
	books[3].Month = 12

	sortByPublished(books)

	// Expected order: 2018/1, 2019/12, 2020/3, 2020/6
	expectedOrder := []string{"2", "4", "3", "1"}
	for i, expectedID := range expectedOrder {
		if books[i].ID != expectedID {
			t.Errorf("Position %d: expected ID %s, got %s", i, expectedID, books[i].ID)
		}
	}
}

// TestLimitByCount tests limiting by book count
func TestLimitByCount(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),
		createTestBook("2", "Batman", 2020, 1, "2", 20, 0),
		createTestBook("3", "Batman", 2020, 1, "3", 20, 0),
		createTestBook("4", "Batman", 2020, 1, "4", 20, 0),
		createTestBook("5", "Batman", 2020, 1, "5", 20, 0),
	}

	result := limitByCount(books, 3)

	if len(result) != 3 {
		t.Errorf("Expected 3 books, got %d", len(result))
	}

	// Should get first 3 books
	for i := 0; i < 3; i++ {
		if result[i].ID != books[i].ID {
			t.Errorf("Position %d: expected ID %s, got %s", i, books[i].ID, result[i].ID)
		}
	}
}

// TestApplySettings_OnlyUnread tests applying OnlyUnread setting
func TestApplySettings_OnlyUnread(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),  // Unread
		createTestBook("2", "Batman", 2020, 1, "2", 20, 20), // Read
		createTestBook("3", "Batman", 2020, 1, "3", 20, 5),  // Partially read
		createTestBook("4", "Batman", 2020, 1, "4", 20, 20), // Read
	}

	settings := &SharedListSettings{
		OnlyUnread: true,
	}

	result, err := ApplySettings(books, settings)
	if err != nil {
		t.Fatalf("ApplySettings failed: %v", err)
	}

	// Should include only unread and partially read books
	if len(result) != 2 {
		t.Errorf("Expected 2 unread books, got %d", len(result))
	}
}

// TestApplySettings_OnlyUnreadWithKeepLastRead tests OnlyUnread with KeepLastRead
func TestApplySettings_OnlyUnreadWithKeepLastRead(t *testing.T) {
	now := time.Now()
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),  // Unread
		createTestBook("2", "Batman", 2020, 1, "2", 20, 20), // Read (recent)
		createTestBook("3", "Batman", 2020, 1, "3", 20, 5),  // Partially read
		createTestBook("4", "Batman", 2020, 1, "4", 20, 20), // Read (older)
		createTestBook("5", "Batman", 2020, 1, "5", 20, 20), // Read (oldest)
	}
	books[1].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -1)} // 1 day ago
	books[3].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -3)} // 3 days ago
	books[4].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -5)} // 5 days ago

	settings := &SharedListSettings{
		OnlyUnread:   true,
		KeepLastRead: true,
	}

	result, err := ApplySettings(books, settings)
	if err != nil {
		t.Fatalf("ApplySettings failed: %v", err)
	}

	// Should include unread (1, 3) + last 3 read books (2, 4, 5)
	// Total: 5 books
	if len(result) != 5 {
		t.Errorf("Expected 5 books (2 unread + 3 last read), got %d", len(result))
	}

	// Verify we got the read books (2 and 4, but not necessarily 5 since we only keep 3)
	hasBook2 := false
	for _, book := range result {
		if book.ID == "2" {
			hasBook2 = true
		}
	}
	if !hasBook2 {
		t.Error("Expected to include most recently read book (ID 2)")
	}
}

// TestApplySettings_KeepLastReadCount verifies the configured
// KeepLastReadCount is honored, and that omitting it (zero value) falls
// back to the ComicRackCE default of 3.
func TestApplySettings_KeepLastReadCount(t *testing.T) {
	now := time.Now()
	newBooks := func() []*library.ComicBook {
		books := []*library.ComicBook{
			createTestBook("1", "Batman", 2020, 1, "1", 20, 0),  // Unread
			createTestBook("2", "Batman", 2020, 1, "2", 20, 20), // Read (most recent)
			createTestBook("3", "Batman", 2020, 1, "3", 20, 20), // Read
			createTestBook("4", "Batman", 2020, 1, "4", 20, 20), // Read
			createTestBook("5", "Batman", 2020, 1, "5", 20, 20), // Read (oldest)
		}
		books[1].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -1)}
		books[2].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -2)}
		books[3].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -3)}
		books[4].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -4)}
		return books
	}

	tests := []struct {
		name       string
		count      int
		wantTotal  int // unread (1 book) + kept read books
		wantKeptID string
	}{
		{name: "explicit count of 1", count: 1, wantTotal: 2, wantKeptID: "2"},
		{name: "explicit count of 2", count: 2, wantTotal: 3, wantKeptID: "3"},
		{name: "zero falls back to default of 3", count: 0, wantTotal: 4, wantKeptID: "4"},
		{name: "negative falls back to default of 3", count: -1, wantTotal: 4, wantKeptID: "4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &SharedListSettings{
				OnlyUnread:        true,
				KeepLastRead:      true,
				KeepLastReadCount: tt.count,
			}

			result, err := ApplySettings(newBooks(), settings)
			if err != nil {
				t.Fatalf("ApplySettings failed: %v", err)
			}
			if len(result) != tt.wantTotal {
				t.Errorf("Expected %d books, got %d", tt.wantTotal, len(result))
			}

			var haveOldestKept bool
			for _, book := range result {
				if book.ID == tt.wantKeptID {
					haveOldestKept = true
				}
			}
			if !haveOldestKept {
				t.Errorf("Expected kept books to include the %s-th most recently read book (ID %s)", tt.name, tt.wantKeptID)
			}
		})
	}
}

// TestEffectiveKeepLastReadCount verifies the standalone helper used for
// display purposes matches ApplySettings' own fallback behavior.
func TestEffectiveKeepLastReadCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  int
	}{
		{name: "explicit positive value", count: 5, want: 5},
		{name: "zero falls back to 3", count: 0, want: 3},
		{name: "negative falls back to 3", count: -2, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &SharedListSettings{KeepLastReadCount: tt.count}
			if got := EffectiveKeepLastReadCount(settings); got != tt.want {
				t.Errorf("EffectiveKeepLastReadCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestApplySettings_SortAndLimit tests sorting followed by limiting
func TestApplySettings_SortAndLimit(t *testing.T) {
	books := []*library.ComicBook{
		createTestBook("1", "Superman", 2020, 1, "1", 20, 0),
		createTestBook("2", "Batman", 2020, 1, "3", 20, 0),
		createTestBook("3", "Batman", 2020, 1, "1", 20, 0),
		createTestBook("4", "Aquaman", 2020, 1, "1", 20, 0),
		createTestBook("5", "Batman", 2020, 1, "2", 20, 0),
	}

	settings := &SharedListSettings{
		Sort:           true,
		ListSortType:   SortTypeSeries,
		Limit:          true,
		LimitValue:     3,
		LimitValueType: LimitTypeBooks,
	}

	result, err := ApplySettings(books, settings)
	if err != nil {
		t.Fatalf("ApplySettings failed: %v", err)
	}

	// Should sort by series, then limit to first 3
	// Order after sort: Aquaman, Batman #1, Batman #2, Batman #3, Superman
	// After limit: Aquaman, Batman #1, Batman #2
	if len(result) != 3 {
		t.Errorf("Expected 3 books after limit, got %d", len(result))
	}

	expectedIDs := []string{"4", "3", "5"} // Aquaman, Batman #1, Batman #2
	for i, expectedID := range expectedIDs {
		if result[i].ID != expectedID {
			t.Errorf("Position %d: expected ID %s, got %s", i, expectedID, result[i].ID)
		}
	}
}

// TestApplySettings_ComplexCombination tests a complex combination of options
func TestApplySettings_ComplexCombination(t *testing.T) {
	now := time.Now()
	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),  // Unread, checked
		createTestBook("2", "Batman", 2019, 1, "2", 20, 20), // Read, checked
		createTestBook("3", "Batman", 2021, 1, "3", 20, 5),  // Partially read, checked
		createTestBook("4", "Batman", 2018, 1, "4", 20, 0),  // Unread, not checked
		createTestBook("5", "Batman", 2022, 1, "5", 20, 0),  // Unread, checked
	}
	books[0].Checked = true
	books[1].Checked = true
	books[1].OpenedTime = library.ComicTime{Time: now.AddDate(0, 0, -1)}
	books[2].Checked = true
	books[4].Checked = true

	settings := &SharedListSettings{
		OnlyUnread:     true,
		OnlyChecked:    true,
		Sort:           true,
		ListSortType:   SortTypePublished,
		Limit:          true,
		LimitValue:     2,
		LimitValueType: LimitTypeBooks,
	}

	result, err := ApplySettings(books, settings)
	if err != nil {
		t.Fatalf("ApplySettings failed: %v", err)
	}

	// Filter: only checked (1,2,3,5) -> only unread (1,3,5)
	// Sort by published: 2020, 2021, 2022
	// Limit to 2: 2020, 2021
	if len(result) != 2 {
		t.Errorf("Expected 2 books, got %d", len(result))
	}

	if result[0].ID != "1" {
		t.Errorf("First book should be ID 1 (2020), got %s", result[0].ID)
	}
	if result[1].ID != "3" {
		t.Errorf("Second book should be ID 3 (2021), got %s", result[1].ID)
	}
}

// TestDefaultSettings tests that default settings are reasonable
func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()

	if settings.OnlyUnread {
		t.Error("Default OnlyUnread should be false")
	}
	if settings.KeepLastRead {
		t.Error("Default KeepLastRead should be false")
	}
	if settings.OnlyChecked {
		t.Error("Default OnlyChecked should be false")
	}
	if settings.Limit {
		t.Error("Default Limit should be false")
	}
	if settings.LimitValue != 50 {
		t.Errorf("Default LimitValue should be 50, got %d", settings.LimitValue)
	}
	if settings.LimitValueType != LimitTypeBooks {
		t.Errorf("Default LimitValueType should be Books, got %v", settings.LimitValueType)
	}
	if !settings.Sort {
		t.Error("Default Sort should be true")
	}
	if settings.ListSortType != SortTypeSeries {
		t.Errorf("Default ListSortType should be Series, got %v", settings.ListSortType)
	}
}

// TestLimitBySize tests limiting by file size
func TestLimitBySize(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()

	// Create test files with known sizes
	file1 := filepath.Join(tmpDir, "book1.cbz")
	file2 := filepath.Join(tmpDir, "book2.cbz")
	file3 := filepath.Join(tmpDir, "book3.cbz")

	// Write files with specific sizes (1MB, 2MB, 3MB)
	writeFile(t, file1, 1*1024*1024)
	writeFile(t, file2, 2*1024*1024)
	writeFile(t, file3, 3*1024*1024)

	books := []*library.ComicBook{
		createTestBook("1", "Batman", 2020, 1, "1", 20, 0),
		createTestBook("2", "Batman", 2020, 1, "2", 20, 0),
		createTestBook("3", "Batman", 2020, 1, "3", 20, 0),
	}
	books[0].FilePath = file1
	books[1].FilePath = file2
	books[2].FilePath = file3

	// Limit to 4MB - should get books 1 and 2 (3MB total)
	result, err := limitBySize(books, 4*1024*1024)
	if err != nil {
		t.Fatalf("limitBySize failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 books (1MB + 2MB < 4MB), got %d", len(result))
	}

	if result[0].ID != "1" || result[1].ID != "2" {
		t.Errorf("Expected books 1 and 2, got %s and %s", result[0].ID, result[1].ID)
	}
}

// Helper function to write a file of specific size
func writeFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer f.Close()

	// Write size bytes
	data := make([]byte, size)
	_, err = f.Write(data)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
}

// TestLimitType_String tests string representation
func TestLimitType_String(t *testing.T) {
	tests := []struct {
		limitType LimitType
		want      string
	}{
		{LimitTypeBooks, "books"},
		{LimitTypeMB, "mb"},
		{LimitTypeGB, "gb"},
		{LimitType(999), "unknown"},
	}

	for _, tt := range tests {
		got := tt.limitType.String()
		if got != tt.want {
			t.Errorf("LimitType(%d).String() = %q, want %q", tt.limitType, got, tt.want)
		}
	}
}

// TestSortType_String tests string representation
func TestSortType_String(t *testing.T) {
	tests := []struct {
		sortType SortType
		want     string
	}{
		{SortTypeSeries, "series"},
		{SortTypeRandom, "random"},
		{SortTypePublished, "published"},
		{SortTypeAdded, "added"},
		{SortTypeStoryArc, "story-arc"},
		{SortTypeListOrder, "list-order"},
		{SortTypeAlternateSeries, "alternate-series"},
		{SortType(999), "unknown"},
	}

	for _, tt := range tests {
		got := tt.sortType.String()
		if got != tt.want {
			t.Errorf("SortType(%d).String() = %q, want %q", tt.sortType, got, tt.want)
		}
	}
}
