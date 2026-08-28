package sync

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// buildTestZip constructs an in-memory zip archive from name->content pairs,
// for tests exercising stripEmbeddedComicInfo without needing a real
// comic archive fixture on disk.
func buildTestZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %q: %v", name, err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("failed to write zip entry %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	return buf.Bytes()
}

// readTestZip returns every entry's decompressed content, keyed by name.
func readTestZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed to open result as zip: %v", err)
	}
	out := make(map[string][]byte)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open entry %q: %v", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("failed to read entry %q: %v", f.Name, err)
		}
		rc.Close()
		out[f.Name] = content
	}
	return out
}

// TestStripEmbeddedComicInfo_RemovesComicInfoXML covers comic-server-oqf's
// actual root cause: real scanner-released .cbz archives can carry an
// embedded ComicInfo.xml whose <Pages> list doesn't match its own
// <PageCount> (one real example had only 2 of 24 declared pages listed).
// ComicRackCE's reference client never embeds this file at all
// (EmbedComicInfo = false, unconditional) - matched here by stripping it
// unconditionally too, not by trying to detect malformed content.
func TestStripEmbeddedComicInfo_RemovesComicInfoXML(t *testing.T) {
	pageData := []byte("fake jpeg bytes for page 1")
	input := buildTestZip(t, map[string][]byte{
		"ComicInfo.xml": []byte(`<ComicInfo><PageCount>24</PageCount></ComicInfo>`),
		"P00001.jpg":    pageData,
	})

	got, err := stripEmbeddedComicInfo(input)
	if err != nil {
		t.Fatalf("stripEmbeddedComicInfo() error = %v", err)
	}

	entries := readTestZip(t, got)
	if _, exists := entries["ComicInfo.xml"]; exists {
		t.Error("expected ComicInfo.xml to be stripped, but it's still present")
	}
	if string(entries["P00001.jpg"]) != string(pageData) {
		t.Errorf("page image content changed: got %q, want %q", entries["P00001.jpg"], pageData)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 remaining entry, got %d: %v", len(entries), entries)
	}
}

// TestStripEmbeddedComicInfo_RemovesComicBookXML covers the same treatment
// for ComicBook.xml (also never embedded by ComicRackCE's reference client).
func TestStripEmbeddedComicInfo_RemovesComicBookXML(t *testing.T) {
	input := buildTestZip(t, map[string][]byte{
		"ComicBook.xml": []byte(`<ComicBook />`),
		"P00001.jpg":    []byte("page data"),
	})

	got, err := stripEmbeddedComicInfo(input)
	if err != nil {
		t.Fatalf("stripEmbeddedComicInfo() error = %v", err)
	}

	entries := readTestZip(t, got)
	if _, exists := entries["ComicBook.xml"]; exists {
		t.Error("expected ComicBook.xml to be stripped, but it's still present")
	}
}

// TestStripEmbeddedComicInfo_NoOpWhenAbsent confirms an archive with no
// embedded metadata file is returned completely unchanged (byte-identical),
// not just functionally equivalent - avoids an unnecessary zip rewrite for
// the common case where there's nothing to strip.
func TestStripEmbeddedComicInfo_NoOpWhenAbsent(t *testing.T) {
	input := buildTestZip(t, map[string][]byte{
		"P00001.jpg": []byte("page data"),
		"P00002.jpg": []byte("more page data"),
	})

	got, err := stripEmbeddedComicInfo(input)
	if err != nil {
		t.Fatalf("stripEmbeddedComicInfo() error = %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Error("expected byte-identical passthrough when no ComicInfo.xml/ComicBook.xml is present")
	}
}

// TestStripEmbeddedComicInfo_MatchesByBaseNameRegardlessOfPath covers a
// ComicInfo.xml nested inside a subfolder within the archive (some real
// scanner releases nest every entry, including ComicInfo.xml, under a
// release-named folder) - matching must be by basename, not exact path.
func TestStripEmbeddedComicInfo_MatchesByBaseNameRegardlessOfPath(t *testing.T) {
	input := buildTestZip(t, map[string][]byte{
		"Release Folder/ComicInfo.xml": []byte(`<ComicInfo />`),
		"Release Folder/P00001.jpg":    []byte("page data"),
	})

	got, err := stripEmbeddedComicInfo(input)
	if err != nil {
		t.Fatalf("stripEmbeddedComicInfo() error = %v", err)
	}

	entries := readTestZip(t, got)
	if _, exists := entries["Release Folder/ComicInfo.xml"]; exists {
		t.Error("expected nested ComicInfo.xml to be stripped, but it's still present")
	}
	if _, exists := entries["Release Folder/P00001.jpg"]; !exists {
		t.Error("expected the nested page image to survive stripping")
	}
}

// TestStripEmbeddedComicInfo_InvalidZipReturnsError confirms a caller can
// distinguish "not a valid zip" from "valid zip, nothing to strip" -
// readComicFile relies on this to soft-fail (ship the archive unmodified)
// rather than corrupt or reject a non-zip source (e.g. a mislabeled file).
func TestStripEmbeddedComicInfo_InvalidZipReturnsError(t *testing.T) {
	if _, err := stripEmbeddedComicInfo([]byte("not a zip file at all")); err == nil {
		t.Error("expected an error for invalid zip data, got nil")
	}
}

// TestReadComicFile_StripsComicInfoFromCBZ is the end-to-end regression
// test for comic-server-oqf: a real .cbz read through readComicFile has
// its embedded ComicInfo.xml stripped, while the page content survives
// unchanged.
func TestReadComicFile_StripsComicInfoFromCBZ(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "Aquamen #1.cbz")
	pageData := []byte("fake page 1 jpeg bytes")
	archive := buildTestZip(t, map[string][]byte{
		"ComicInfo.xml": []byte(`<ComicInfo><PageCount>24</PageCount><Pages><Page Image="0"/></Pages></ComicInfo>`),
		"P00001.jpg":    pageData,
	})
	if err := os.WriteFile(realPath, archive, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	syncer := NewSyncer(nil, nil)
	book := &library.ComicBook{ID: "1", FilePath: realPath}
	got, err := syncer.readComicFile(book)
	if err != nil {
		t.Fatalf("readComicFile() error = %v", err)
	}

	entries := readTestZip(t, got)
	if _, exists := entries["ComicInfo.xml"]; exists {
		t.Error("expected readComicFile to strip embedded ComicInfo.xml for a .cbz")
	}
	if string(entries["P00001.jpg"]) != string(pageData) {
		t.Errorf("page image content changed: got %q, want %q", entries["P00001.jpg"], pageData)
	}
}

// TestReadComicFile_DoesNotTouchNonCBZExtensions confirms the strip only
// applies to .cbz - a .cbr/.cb7 (rar/7z, not zip) fed through the same
// path must come back byte-identical, not attempted-and-failed-to-parse.
func TestReadComicFile_DoesNotTouchNonCBZExtensions(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "Batman #1.cbr")
	want := []byte("fake rar contents, not a zip")
	if err := os.WriteFile(realPath, want, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	syncer := NewSyncer(nil, nil)
	book := &library.ComicBook{ID: "1", FilePath: realPath}
	got, err := syncer.readComicFile(book)
	if err != nil {
		t.Fatalf("readComicFile() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("readComicFile() = %q, want %q (should be untouched for non-.cbz)", got, want)
	}
}

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
				"book1.cbp.xml": []byte(`<?xml version="1.0"?><Book Id="book1"><Title>Test Book</Title></Book>`),
			},
			listResult:    "book1.cbp\nbook1.cbp.xml",
			expectedBooks: 1,
			expectError:   false,
		},
		{
			name: "multiple books with sidecars",
			deviceFiles: map[string][]byte{
				"book1.cbp":     []byte("comic data 1"),
				"book1.cbp.xml": []byte(`<?xml version="1.0"?><Book Id="book1"><Title>Book 1</Title></Book>`),
				"book2.cbp":     []byte("comic data 2"),
				"book2.cbp.xml": []byte(`<?xml version="1.0"?><Book Id="book2"><Title>Book 2</Title></Book>`),
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
			backend := library.NewXMLBackendFromLibrary(lib, "", nil)
			syncer := NewSyncer(mockClient, backend)

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

// TestGetDeviceBooks_CircuitBreaksOnUnreachableDevice is the regression
// test for a real lockout found live 2026-08-27: when a device goes
// completely unreachable mid-sync (not just slow - every single read
// fails), the individual-sidecar-read fallback used to grind through
// every remaining file (each with its own 3 retries) with no way to bail
// out early. Since a device can't start a second sync while one is
// already running (StartSync rejects it), that meant a hung, doomed sync
// against an unreachable device blocked the user from retrying for as
// long as it took to exhaust the whole file list - potentially thousands
// of files. This asserts GetDeviceBooks now fails fast instead of trying
// every file.
func TestGetDeviceBooks_CircuitBreaksOnUnreachableDevice(t *testing.T) {
	client := NewMockClient()

	// 30 fake device files - many more than the circuit breaker's
	// threshold, so a naive implementation would try all 30 (x3 retries
	// each = 90 ReadFile calls).
	var fileList string
	for i := 0; i < 30; i++ {
		if i > 0 {
			fileList += "\n"
		}
		fileList += fmt.Sprintf("book%d.cbp", i)
	}
	client.ListFilesResult = fileList
	client.ReadMultiError = fmt.Errorf("batch read unsupported") // force the individual-read fallback
	client.ReadFileError = fmt.Errorf("connection refused")      // every individual read fails too

	backend := library.NewXMLBackendFromLibrary(&library.ComicLibrary{}, "", nil)
	syncer := NewSyncer(client, backend)

	_, err := syncer.GetDeviceBooks()
	if err == nil {
		t.Fatal("expected GetDeviceBooks to fail fast against a completely unreachable device")
	}

	// 10 (circuit breaker threshold) x 3 (retries per file) = 30 calls,
	// not 90 (which is what trying all 30 files would produce).
	if len(client.ReadFileCalls) > 30 {
		t.Errorf("expected GetDeviceBooks to bail out well before trying all files, got %d ReadFile calls", len(client.ReadFileCalls))
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
			backend := library.NewXMLBackendFromLibrary(lib, "", nil)
			syncer := NewSyncer(mockClient, backend)

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
			expected: false, // Sparse vs full page metadata - not a real change when PageCount matches
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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(mockClient, backend)

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
			{
				ID:   "list4",
				Name: "A Folder",
				Type: "ComicListItemFolder",
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

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

	// A reading list has real book membership - it should be accepted
	// alongside a smart list (comic-server-vwl's device-sync fix: any list
	// type with books works, not just smart lists).
	readingList := &lib.ComicLists[2]
	if err := syncer.SetFilterLists([]*library.ComicListItem{smartList1, readingList}); err != nil {
		t.Errorf("expected a reading list to be accepted, got error: %v", err)
	}

	// A folder groups other lists rather than containing books itself -
	// it should still be rejected.
	folder := &lib.ComicLists[3]
	if err := syncer.SetFilterLists([]*library.ComicListItem{smartList1, folder}); err == nil {
		t.Error("expected error when setting a folder, got nil")
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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	// Set multiple filter lists
	err := syncer.SetFilterLists([]*library.ComicListItem{
		&lib.ComicLists[0],
		&lib.ComicLists[1],
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compute union
	books, err := syncer.computeUnionOfLists()
	if err != nil {
		t.Fatalf("computeUnionOfLists() error = %v", err)
	}

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

// TestComputeSyncPlan_IdListSyncsItsBooks is the regression test for
// comic-server-vwl's device-sync fix: an ID list ("To Read",
// ComicIdListItem) assigned as a device's filter list must actually sync
// its books. Before the fix, computeUnionOfLists called
// backend.MatchBooks, which errors on anything that isn't a smart list -
// silently dropping the list from the union (or, via
// cmd/server.go's applyDeviceConfig, aborting the device's sync outright)
// even though SetFilterLists itself now accepts the list.
func TestComputeSyncPlan_IdListSyncsItsBooks(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: "/path/book1.cbz"},
			{ID: "book2", Title: "Book 2", FilePath: "/path/book2.cbz"},
			{ID: "book3", Title: "Book 3", FilePath: "/path/book3.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:      "idlist1",
				Name:    "To Read",
				Type:    "ComicIdListItem",
				BookIds: []string{"book1", "book3"},
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	if err := syncer.SetFilterLists([]*library.ComicListItem{&lib.ComicLists[0]}); err != nil {
		t.Fatalf("unexpected error setting an ID list as a filter: %v", err)
	}

	operations, err := syncer.ComputeSyncPlan(make(map[string]*DeviceBook))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bookIDs := make(map[string]bool)
	for _, op := range operations {
		bookIDs[op.Book.ID] = true
	}
	if !bookIDs["book1"] || !bookIDs["book3"] {
		t.Errorf("expected book1 and book3 (the ID list's members) to sync, got operations: %+v", operations)
	}
	if bookIDs["book2"] {
		t.Error("book2 is not in the ID list and should not sync")
	}
}

// TestGetReadingLists_NoDuplicateEntryForNonSmartFilterList is the
// regression test for a real device bug found live 2026-08-27: an ID
// list ("To Read", ComicIdListItem) assigned as a device's filter list
// showed its books synced fine, but the list itself never appeared on
// the device. getReadingLists' "regular reading lists" pass only skipped
// smart lists (comicrack:ComicSmartListItem), so a filter list of any
// other type (ID list, reading list) got written into sync_information.xml
// TWICE: once here with its raw, unfiltered ComicReadingListItem Items
// (empty for an ID list, since those store membership in BookIds instead)
// and once more, correctly, in the filter-list pass below. Two <List>
// entries sharing a Name - the first one empty - is exactly the kind of
// thing a device app would resolve by hiding the list.
func TestGetReadingLists_NoDuplicateEntryForNonSmartFilterList(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: "/path/book1.cbz"},
			{ID: "book2", Title: "Book 2", FilePath: "/path/book2.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:      "idlist1",
				Name:    "To Read",
				Type:    "ComicIdListItem",
				BookIds: []string{"book1", "book2"},
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	if err := syncer.SetFilterLists([]*library.ComicListItem{&lib.ComicLists[0]}); err != nil {
		t.Fatalf("unexpected error setting an ID list as a filter: %v", err)
	}

	readingLists := syncer.getReadingLists()

	var matches []ReadingList
	for _, rl := range readingLists {
		if rl.Name == "To Read" {
			matches = append(matches, rl)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 \"To Read\" entry in sync_information.xml, got %d: %+v", len(matches), matches)
	}
	if matches[0].Books == nil || len(matches[0].Books.ID) != 2 {
		t.Errorf("expected the single \"To Read\" entry to have its 2 real books, got %+v", matches[0])
	}
}

// TestGetReadingLists_OmitsEmptyFilterList covers comic-server-lev:
// ComicRackCE's own SetLists (StorageSync.cs) never serializes a reading
// list with zero books at all - `where cli.Books.Count > 0` filters it out
// before the XML is even built. comic-server used to still write a <List>
// entry (just without a <Books> child) whenever a filter list's settings
// (e.g. only-unread) filtered every book out. That shape was never part of
// what the Android app was built against.
func TestGetReadingLists_OmitsEmptyFilterList(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: "/path/book1.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:      "idlist1",
				Name:    "Empty List",
				Type:    "ComicIdListItem",
				BookIds: []string{}, // resolves to zero books
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	if err := syncer.SetFilterLists([]*library.ComicListItem{&lib.ComicLists[0]}); err != nil {
		t.Fatalf("unexpected error setting an ID list as a filter: %v", err)
	}

	readingLists := syncer.getReadingLists()
	for _, rl := range readingLists {
		if rl.Name == "Empty List" {
			t.Fatalf("expected no entry for a list with zero books, got %+v", rl)
		}
	}
}

// TestGetReadingLists_OmitsEmptyRegularList covers the same rule
// (comic-server-lev) for the "regular reading lists" pass, not just the
// filter-list pass.
func TestGetReadingLists_OmitsEmptyRegularList(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: "/path/book1.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:    "readinglist1",
				Name:  "Empty Reading List",
				Type:  "ComicReadingList",
				Items: nil, // resolves to zero books
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	readingLists := syncer.getReadingLists()
	for _, rl := range readingLists {
		if rl.Name == "Empty Reading List" {
			t.Fatalf("expected no entry for a list with zero books, got %+v", rl)
		}
	}
}

// TestGetReadingLists_OmitsFolders covers comic-server-lev: folder items
// (e.g. the built-in "Library"/"Smart Lists"/"Temporary Lists" tree nodes)
// group other lists rather than containing books themselves, and must
// never be written into sync_information.xml as if they were reading
// lists. Found live on a real device: three such empty <List> entries
// landed ahead of the real lists in the file, a shape ComicRackCE's own
// producer never emits (it filters 0-book lists before serializing).
func TestGetReadingLists_OmitsFolders(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			{ID: "book1", Title: "Book 1", FilePath: "/path/book1.cbz"},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:   "folder1",
				Name: "Library",
				Type: "comicrack:ComicListItemFolder",
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	readingLists := syncer.getReadingLists()
	for _, rl := range readingLists {
		if rl.Name == "Library" {
			t.Fatalf("expected no entry for a folder, got %+v", rl)
		}
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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

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

// TestComputeSyncPlan_PerListSettings is the regression test for
// comic-server-3oq: before SetFilterListsWithSettings existed, every list
// assigned to a device silently shared ONE settings object the moment a
// device had more than one enabled list, discarding any per-list
// only-unread/limit/sort configuration outright. Two lists here each
// carry their own settings ("Unread Only" filters out a read book, "All
// Books" doesn't) and the test asserts each list's own setting actually
// takes effect on its own books.
func TestComputeSyncPlan_PerListSettings(t *testing.T) {
	lib := &library.ComicLibrary{
		Books: []library.ComicBook{
			// Series A: one read, one unread - only the unread one should
			// survive "Unread Only" on list1.
			{ID: "book1-read", Series: "Series A", Title: "Book 1 (read)", FilePath: "/path/book1.cbz", PageCount: 20, CurrentPage: 20, OpenCount: 1, LastPageRead: 19},
			{ID: "book2-unread", Series: "Series A", Title: "Book 2 (unread)", FilePath: "/path/book2.cbz", PageCount: 20, CurrentPage: 0},
			// Series B: one read - list2 has no only-unread filter, so it
			// should still come through.
			{ID: "book3-read", Series: "Series B", Title: "Book 3 (read)", FilePath: "/path/book3.cbz", PageCount: 20, CurrentPage: 20, OpenCount: 1, LastPageRead: 19},
		},
		ComicLists: []library.ComicListItem{
			{
				ID:          "list1",
				Name:        "Series A (unread only)",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series A"},
				},
			},
			{
				ID:          "list2",
				Name:        "Series B (all)",
				Type:        "ComicSmartListItem",
				MatcherMode: "And",
				Matchers: []library.ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Series B"},
				},
			},
		},
	}

	client := NewMockClient()
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

	unreadOnly := &SharedListSettings{OnlyUnread: true}
	err := syncer.SetFilterListsWithSettings([]FilterListEntry{
		{List: &lib.ComicLists[0], Settings: unreadOnly},
		{List: &lib.ComicLists[1], Settings: nil}, // falls back to syncer default (no filtering)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	operations, err := syncer.ComputeSyncPlan(make(map[string]*DeviceBook))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bookIDs := make(map[string]bool)
	for _, op := range operations {
		bookIDs[op.Book.ID] = true
	}

	if bookIDs["book1-read"] {
		t.Error("book1-read should have been excluded by list1's only-unread setting")
	}
	if !bookIDs["book2-unread"] {
		t.Error("book2-unread should have been included (unread, matches list1)")
	}
	if !bookIDs["book3-read"] {
		t.Error("book3-read should have been included (list2 has no only-unread filter)")
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
	backend := library.NewXMLBackendFromLibrary(lib, "", nil)
	syncer := NewSyncer(client, backend)

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

// TestReadComicFile_UsesPathResolver covers comic-server-4n9: a library
// authored on a different OS/mount (e.g. Windows paths served from a Linux
// container) needs its raw FilePath translated before the actual file
// transfer reads it - the exact bug class comic-server-ivq fixed for cover
// extraction, confirmed here for the real device-sync file read too.
func TestReadComicFile_UsesPathResolver(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "Batman #1.cbz")
	want := []byte("fake cbz contents")
	if err := os.WriteFile(realPath, want, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	syncer := NewSyncer(nil, nil)
	syncer.SetPathResolver(func(rawPath string) string {
		if rawPath == `G:\Comics\Batman\Batman #1.cbz` {
			return realPath
		}
		return rawPath
	})

	book := &library.ComicBook{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`}
	got, err := syncer.readComicFile(book)
	if err != nil {
		t.Fatalf("readComicFile() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("readComicFile() = %q, want %q", got, want)
	}
}

// TestReadComicFile_NoResolverConfigured confirms the default (identity)
// resolver preserves existing behavior for the common case: a library
// whose raw FilePath is already directly readable.
func TestReadComicFile_NoResolverConfigured(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "Batman #1.cbz")
	want := []byte("fake cbz contents")
	if err := os.WriteFile(realPath, want, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	syncer := NewSyncer(nil, nil)
	book := &library.ComicBook{ID: "1", FilePath: realPath}
	got, err := syncer.readComicFile(book)
	if err != nil {
		t.Fatalf("readComicFile() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("readComicFile() = %q, want %q", got, want)
	}
}

// TestCalculateRequiredSpace_UsesPathResolver covers the free-space check
// that runs before every sync - it must also translate the book's path or
// it silently falls back to a 50MB-per-book estimate for every book,
// masking the real sizes.
func TestCalculateRequiredSpace_UsesPathResolver(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "Batman #1.cbz")
	if err := os.WriteFile(realPath, make([]byte, 12345), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	syncer := NewSyncer(nil, nil)
	syncer.SetPathResolver(func(rawPath string) string {
		if rawPath == `G:\Comics\Batman\Batman #1.cbz` {
			return realPath
		}
		return rawPath
	})

	ops := []SyncOperation{
		{Type: OperationAdd, Book: &library.ComicBook{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`}},
	}
	total, err := syncer.calculateRequiredSpace(ops)
	if err != nil {
		t.Fatalf("calculateRequiredSpace() error = %v", err)
	}
	// Must reflect the real 12345-byte file, not the 50MB fallback estimate
	// used when the path can't be resolved.
	if total < 12345 || total > 12345+1024*1024 {
		t.Errorf("calculateRequiredSpace() = %d, expected close to the real 12345-byte file size, not a fallback estimate", total)
	}
}
