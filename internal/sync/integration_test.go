package sync

import (
	"encoding/xml"
	"errors"
	"os"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestPerformSync_EmptyLibraryAndDevice tests sync with no books
func TestPerformSync_EmptyLibraryAndDevice(t *testing.T) {
	mockClient := NewMockClient()
	lib := &library.ComicLibrary{Books: []library.ComicBook{}}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if result.BooksAdded != 0 {
		t.Errorf("expected 0 books added, got %d", result.BooksAdded)
	}
	if result.BooksUpdated != 0 {
		t.Errorf("expected 0 books updated, got %d", result.BooksUpdated)
	}
	if result.BooksDeleted != 0 {
		t.Errorf("expected 0 books deleted, got %d", result.BooksDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}

	// Verify protocol sequence
	if mockClient.SendStartCalls != 1 {
		t.Errorf("expected 1 SendStart call, got %d", mockClient.SendStartCalls)
	}
	if mockClient.GetDeviceInfoCalls != 1 {
		t.Errorf("expected 1 GetDeviceInfo call, got %d", mockClient.GetDeviceInfoCalls)
	}
	if mockClient.GetFreeSpaceCalls != 1 {
		t.Errorf("expected 1 GetFreeSpace call, got %d", mockClient.GetFreeSpaceCalls)
	}
	if mockClient.ListFilesCalls != 1 {
		t.Errorf("expected 1 ListFiles call, got %d", mockClient.ListFilesCalls)
	}
	if mockClient.SendCompletedCalls != 1 {
		t.Errorf("expected 1 SendCompleted call, got %d", mockClient.SendCompletedCalls)
	}

	// Verify sync_information.xml was written
	_, exists := mockClient.GetFile("sync_information.xml")
	if !exists {
		t.Error("expected sync_information.xml to be written")
	}
}

// TestPerformSync_AddBooks tests adding new books to device
func TestPerformSync_AddBooks(t *testing.T) {
	// Create temporary comic book files
	tmpDir := t.TempDir()
	book1Path := tmpDir + "/book1.cbz"
	book2Path := tmpDir + "/book2.cbz"

	// Write dummy comic book data
	if err := os.WriteFile(book1Path, []byte("dummy comic 1 data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(book2Path, []byte("dummy comic 2 data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	mockClient := NewMockClient()
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", PageCount: 10, FilePath: book1Path},
			{ID: "book2", Title: "Book 2", PageCount: 15, FilePath: book2Path},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if result.BooksAdded != 2 {
		t.Errorf("expected 2 books added, got %d", result.BooksAdded)
	}
	if result.BooksUpdated != 0 {
		t.Errorf("expected 0 books updated, got %d", result.BooksUpdated)
	}
	if result.BooksDeleted != 0 {
		t.Errorf("expected 0 books deleted, got %d", result.BooksDeleted)
	}

	// Verify sidecars were written
	sidecar1, exists1 := mockClient.GetFile("book1.cbp.xml")
	if !exists1 {
		t.Error("expected book1.cbp.xml to be written")
	}
	sidecar2, exists2 := mockClient.GetFile("book2.cbp.xml")
	if !exists2 {
		t.Error("expected book2.cbp.xml to be written")
	}

	// Verify sidecar content
	var book1 library.ComicBook
	if err := xml.Unmarshal(sidecar1, &book1); err != nil {
		t.Errorf("failed to parse book1 sidecar: %v", err)
	}
	if book1.Title != "Book 1" {
		t.Errorf("expected title 'Book 1', got '%s'", book1.Title)
	}

	var book2 library.ComicBook
	if err := xml.Unmarshal(sidecar2, &book2); err != nil {
		t.Errorf("failed to parse book2 sidecar: %v", err)
	}
	if book2.Title != "Book 2" {
		t.Errorf("expected title 'Book 2', got '%s'", book2.Title)
	}

	// Verify progress updates were sent
	if len(mockClient.SendProgressCalls) < 2 {
		t.Errorf("expected at least 2 progress updates, got %d", len(mockClient.SendProgressCalls))
	}
	// Final progress should be 100%
	finalProgress := mockClient.SendProgressCalls[len(mockClient.SendProgressCalls)-1]
	if finalProgress != 100 {
		t.Errorf("expected final progress to be 100, got %d", finalProgress)
	}
}

// TestPerformSync_DeleteBooks tests deleting books from device
func TestPerformSync_DeleteBooks(t *testing.T) {
	mockClient := NewMockClient()

	// Setup device with existing books
	mockClient.AddFile("book1.cbp", []byte("data"))
	mockClient.AddFile("book1.cbp.xml", []byte(`<?xml version="1.0"?><Book Id="book1"><Title>Book 1</Title></Book>`))
	mockClient.AddFile("book2.cbp", []byte("data"))
	mockClient.AddFile("book2.cbp.xml", []byte(`<?xml version="1.0"?><Book Id="book2"><Title>Book 2</Title></Book>`))
	mockClient.ListFilesResult = "book1.cbp\nbook1.cbp.xml\nbook2.cbp\nbook2.cbp.xml"

	// Library is empty, so all books should be deleted
	lib := &library.ComicLibrary{Books: []library.ComicBook{}}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if result.BooksAdded != 0 {
		t.Errorf("expected 0 books added, got %d", result.BooksAdded)
	}
	if result.BooksUpdated != 0 {
		t.Errorf("expected 0 books updated, got %d", result.BooksUpdated)
	}
	if result.BooksDeleted != 2 {
		t.Errorf("expected 2 books deleted, got %d", result.BooksDeleted)
	}

	// Verify files were deleted
	if len(mockClient.DeleteFileCalls) != 4 { // 2 .cbp + 2 .cbp.xml
		t.Errorf("expected 4 delete calls, got %d", len(mockClient.DeleteFileCalls))
	}
}

// TestPerformSync_UpdateMetadataOnly tests updating only metadata
func TestPerformSync_UpdateMetadataOnly(t *testing.T) {
	mockClient := NewMockClient()

	// Setup device with existing book
	mockClient.AddFile("book1.cbp", []byte("data"))
	mockClient.AddFile("book1.cbp.xml", []byte(`<?xml version="1.0"?><Book Id="book1"><Title>Old Title</Title><PageCount>10</PageCount></Book>`))
	mockClient.ListFilesResult = "book1.cbp\nbook1.cbp.xml"

	// Library has same book with updated title
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "New Title", PageCount: 10},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if result.BooksAdded != 0 {
		t.Errorf("expected 0 books added, got %d", result.BooksAdded)
	}
	if result.BooksUpdated != 1 {
		t.Errorf("expected 1 book updated, got %d", result.BooksUpdated)
	}
	if result.BooksDeleted != 0 {
		t.Errorf("expected 0 books deleted, got %d", result.BooksDeleted)
	}

	// Verify only sidecar was written (not the .cbp file)
	writeCount := 0
	for _, call := range mockClient.WriteFileCalls {
		if call.Filename == "book1.cbp.xml" {
			writeCount++
			// Verify updated metadata
			var book library.ComicBook
			if err := xml.Unmarshal(call.Data, &book); err != nil {
				t.Errorf("failed to parse updated sidecar: %v", err)
			}
			if book.Title != "New Title" {
				t.Errorf("expected title 'New Title', got '%s'", book.Title)
			}
		}
	}
	if writeCount != 1 {
		t.Errorf("expected 1 write to book1.cbp.xml, got %d", writeCount)
	}
}

// TestPerformSync_MixedOperations tests a complex sync scenario
func TestPerformSync_MixedOperations(t *testing.T) {
	// Create temporary comic book files for books that need to be added
	tmpDir := t.TempDir()
	book4Path := tmpDir + "/book4.cbz"
	if err := os.WriteFile(book4Path, []byte("dummy comic 4 data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	mockClient := NewMockClient()

	// Setup device with existing books
	mockClient.AddFile("book1.cbp", []byte("data"))
	mockClient.AddFile("book1.cbp.xml", []byte(`<?xml version="1.0"?><Book Id="book1"><Title>Unchanged</Title><PageCount>10</PageCount></Book>`))
	mockClient.AddFile("book2.cbp", []byte("data"))
	mockClient.AddFile("book2.cbp.xml", []byte(`<?xml version="1.0"?><Book Id="book2"><Title>Old Title</Title><PageCount>10</PageCount></Book>`))
	mockClient.AddFile("book3.cbp", []byte("data"))
	mockClient.AddFile("book3.cbp.xml", []byte(`<?xml version="1.0"?><Book Id="book3"><Title>To Delete</Title></Book>`))
	mockClient.ListFilesResult = "book1.cbp\nbook1.cbp.xml\nbook2.cbp\nbook2.cbp.xml\nbook3.cbp\nbook3.cbp.xml"

	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Unchanged", PageCount: 10},                    // No change
			{ID: "book2", Title: "New Title", PageCount: 10},                    // Metadata update
			{ID: "book4", Title: "New Book", PageCount: 5, FilePath: book4Path}, // Add
		},
		// book3 is not in library -> Delete
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if result.BooksAdded != 1 { // book4
		t.Errorf("expected 1 book added, got %d", result.BooksAdded)
	}
	if result.BooksUpdated != 1 { // book2
		t.Errorf("expected 1 book updated, got %d", result.BooksUpdated)
	}
	if result.BooksDeleted != 1 { // book3
		t.Errorf("expected 1 book deleted, got %d", result.BooksDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

// TestPerformSync_ErrorHandling tests error scenarios
func TestPerformSync_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupError  func(*MockClient)
		expectError bool
		errorMsg    string
	}{
		{
			name: "SendStart fails",
			setupError: func(m *MockClient) {
				m.SendStartError = errors.New("start failed")
			},
			expectError: true,
			errorMsg:    "failed to start sync",
		},
		{
			name: "GetDeviceInfo fails",
			setupError: func(m *MockClient) {
				m.GetInfoError = errors.New("info failed")
			},
			expectError: true,
			errorMsg:    "failed to get device info",
		},
		{
			name: "GetFreeSpace fails",
			setupError: func(m *MockClient) {
				m.FreeSpaceError = errors.New("space check failed")
			},
			expectError: true,
			errorMsg:    "failed to get free space",
		},
		{
			name: "ListFiles fails",
			setupError: func(m *MockClient) {
				m.ListFilesError = errors.New("list failed")
			},
			expectError: true,
			errorMsg:    "failed to list device files",
		},
		{
			name: "SendCompleted fails",
			setupError: func(m *MockClient) {
				m.SendCompletedErr = errors.New("completed failed")
			},
			expectError: true,
			errorMsg:    "failed to send completion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := NewMockClient()
			tt.setupError(mockClient)

			lib := &library.ComicLibrary{Books: []library.ComicBook{}}
			backend := library.NewXMLBackendFromLibrary(lib, "", nil)
			syncer := NewSyncer(mockClient, backend)

			_, err := syncer.PerformSync()

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectError && err != nil {
				if err.Error() == "" {
					t.Error("expected error message")
				}
			}
		})
	}
}

// TestPerformSync_PartialFailures tests handling of individual operation failures
func TestPerformSync_PartialFailures(t *testing.T) {
	// For now, skip this test as it requires custom mock behavior
	// to fail specific operations mid-sync
	t.Skip("Partial failure testing requires more sophisticated mocking")
}

// TestPerformSync_AbortHandling tests sync abort detection
func TestPerformSync_AbortHandling(t *testing.T) {
	mockClient := NewMockClient()

	// Create library with many books to trigger abort check
	books := make([]library.ComicBook, 25)
	for i := 0; i < 25; i++ {
		books[i] = library.ComicBook{
			ID:    string(rune('a' + i)),
			Title: "Book " + string(rune('A'+i)),
		}
	}
	lib := &library.ComicLibrary{Books: books}

	// Set abort after a few operations
	checkCount := 0
	mockClient.CheckAbortFunc = func() (bool, error) {
		checkCount++
		if checkCount > 1 {
			return true, nil // Abort after second check
		}
		return false, nil
	}

	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	_, err := syncer.PerformSync()

	if err == nil {
		t.Error("expected error due to abort")
	}
	if err != nil && err.Error() != "sync aborted by user" {
		t.Errorf("expected 'sync aborted by user' error, got: %v", err)
	}

	// Verify abort was checked
	if mockClient.CheckAbortCalls == 0 {
		t.Error("expected CheckAbort to be called")
	}
}

// TestPerformSync_AbortCheckedEveryOperation is the regression test for
// comic-server-akt: abort used to only be checked every 10th operation, so
// a user-initiated abort could take up to 9 more operations to actually
// register. With a library of 5 books, an abort surfacing on the very
// first CheckAbort call should now stop the sync immediately, not after
// several more operations run first.
func TestPerformSync_AbortCheckedEveryOperation(t *testing.T) {
	mockClient := NewMockClient()

	books := make([]library.ComicBook, 5)
	for i := 0; i < 5; i++ {
		books[i] = library.ComicBook{ID: string(rune('a' + i)), Title: "Book"}
	}
	lib := &library.ComicLibrary{Books: books}

	mockClient.CheckAbortFunc = func() (bool, error) {
		return true, nil // Abort on the very first check
	}

	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err == nil || err.Error() != "sync aborted by user" {
		t.Errorf("expected 'sync aborted by user' error, got: %v", err)
	}
	if mockClient.CheckAbortCalls != 1 {
		t.Errorf("expected exactly 1 CheckAbort call before stopping, got %d", mockClient.CheckAbortCalls)
	}
	if result.BooksAdded != 0 {
		t.Errorf("expected 0 books added before the immediate abort, got %d", result.BooksAdded)
	}
}

// TestPerformSync_AbortCheckFailureStopsSync is the regression test for
// comic-server-akt's second half: a device that can't even answer
// CheckAbort (unresponsive/dropped connection) should stop the sync
// rather than being logged as a warning and left to grind through every
// remaining operation, each individually timing out against a device
// that's already gone.
func TestPerformSync_AbortCheckFailureStopsSync(t *testing.T) {
	mockClient := NewMockClient()

	books := make([]library.ComicBook, 5)
	for i := 0; i < 5; i++ {
		books[i] = library.ComicBook{ID: string(rune('a' + i)), Title: "Book"}
	}
	lib := &library.ComicLibrary{Books: books}

	mockClient.CheckAbortFunc = func() (bool, error) {
		return false, errors.New("i/o timeout")
	}

	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	result, err := syncer.PerformSync()
	if err == nil {
		t.Fatal("expected an error when CheckAbort itself fails")
	}
	if result.BooksAdded != 0 {
		t.Errorf("expected 0 books added when the device is unresponsive, got %d", result.BooksAdded)
	}
}

// TestPerformSync_ProgressUpdates tests that progress is reported correctly
func TestPerformSync_ProgressUpdates(t *testing.T) {
	// Create temporary comic book files
	tmpDir := t.TempDir()
	book1Path := tmpDir + "/book1.cbz"
	book2Path := tmpDir + "/book2.cbz"
	book3Path := tmpDir + "/book3.cbz"
	if err := os.WriteFile(book1Path, []byte("dummy comic 1 data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(book2Path, []byte("dummy comic 2 data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(book3Path, []byte("dummy comic 3 data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	mockClient := NewMockClient()

	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: book1Path},
			{ID: "book2", Title: "Book 2", FilePath: book2Path},
			{ID: "book3", Title: "Book 3", FilePath: book3Path},
		},
	}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	_, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify progress updates were sent
	if len(mockClient.SendProgressCalls) < 3 {
		t.Errorf("expected at least 3 progress updates, got %d", len(mockClient.SendProgressCalls))
	}

	// Verify progress is monotonically increasing
	for i := 1; i < len(mockClient.SendProgressCalls); i++ {
		if mockClient.SendProgressCalls[i] < mockClient.SendProgressCalls[i-1] {
			t.Errorf("progress decreased: %d -> %d",
				mockClient.SendProgressCalls[i-1],
				mockClient.SendProgressCalls[i])
		}
	}

	// Final progress should be 100
	finalProgress := mockClient.SendProgressCalls[len(mockClient.SendProgressCalls)-1]
	if finalProgress != 100 {
		t.Errorf("expected final progress to be 100, got %d", finalProgress)
	}
}

// TestPerformSync_SyncInformationWritten tests that reading lists are written
func TestPerformSync_SyncInformationWritten(t *testing.T) {
	mockClient := NewMockClient()
	lib := &library.ComicLibrary{Books: []library.ComicBook{}}
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

	_, err := syncer.PerformSync()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sync_information.xml was written
	data, exists := mockClient.GetFile("sync_information.xml")
	if !exists {
		t.Fatal("expected sync_information.xml to be written")
	}

	// Verify it's valid XML
	dataStr := string(data)
	if dataStr == "" {
		t.Error("sync_information.xml is empty")
	}
	if !contains(dataStr, "<?xml") {
		t.Error("sync_information.xml missing XML declaration")
	}
	if !contains(dataStr, "<SyncInformation>") {
		t.Error("sync_information.xml missing SyncInformation element")
	}
}

// TestGetTitleForOp tests title extraction from operations
func TestGetTitleForOp(t *testing.T) {
	tests := []struct {
		name     string
		op       SyncOperation
		expected string
	}{
		{
			name: "book with title",
			op: SyncOperation{
				Book: &library.ComicBook{
					ID:    "book1",
					Title: "Test Book",
				},
			},
			expected: "Test Book",
		},
		{
			name: "book without title",
			op: SyncOperation{
				Book: &library.ComicBook{
					ID: "book1",
				},
			},
			expected: "book1",
		},
		{
			name: "device with metadata",
			op: SyncOperation{
				Device: &DeviceBook{
					Filename: "book1.cbp",
					Metadata: &library.ComicBook{
						Title: "Device Book",
					},
				},
			},
			expected: "Device Book",
		},
		{
			name: "device without metadata",
			op: SyncOperation{
				Device: &DeviceBook{
					Filename: "book1.cbp",
				},
			},
			expected: "book1", // .cbp extension is stripped for display
		},
		{
			name:     "empty operation",
			op:       SyncOperation{},
			expected: "(unknown)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTitleForOp(tt.op)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				indexOfSubstring(s, substr) >= 0))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
