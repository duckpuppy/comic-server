package sync

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/protocol"
)

// Client defines the interface for communicating with a ComicRack device
type Client interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte) error
	DeleteFile(filename string) error
	FileExists(filename string) (bool, error)
	ListFiles() (string, error)
	ReadMultiFile(filenames []string) (map[string][]byte, error)
	GetDeviceInfo() (*protocol.DeviceInfo, error)
	SendStart() error
	SendCompleted() error
	SendProgressUpdate(percent int) error
	GetFreeSpace() (int64, error)
	CheckAbort() (bool, error)
}

// Syncer orchestrates synchronization between the library and a device
type Syncer struct {
	client     Client
	library    *library.ComicLibrary
	filterList *library.ComicListItem // Optional smart list to filter books
	settings   *SharedListSettings    // Sync settings to apply (filtering, sorting, limiting)
}

// NewSyncer creates a new sync orchestrator
func NewSyncer(client Client, lib *library.ComicLibrary) *Syncer {
	return &Syncer{
		client:     client,
		library:    lib,
		filterList: nil,
		settings:   DefaultSettings(), // Use default settings
	}
}

// SetFilterList sets a smart list to filter which books get synced
// Pass nil to sync all books
func (s *Syncer) SetFilterList(list *library.ComicListItem) error {
	if list != nil {
		// Validate it's a smart list
		if !strings.Contains(list.Type, "SmartList") {
			return fmt.Errorf("list %q is not a smart list (type: %s)", list.Name, list.Type)
		}
	}
	s.filterList = list
	return nil
}

// SetSettings configures the sync settings (filtering, sorting, limiting)
// Pass nil to use default settings
func (s *Syncer) SetSettings(settings *SharedListSettings) {
	if settings == nil {
		s.settings = DefaultSettings()
	} else {
		s.settings = settings
	}
}

// GetSettings returns the current sync settings
func (s *Syncer) GetSettings() *SharedListSettings {
	return s.settings
}

// SyncResult contains the results of a synchronization operation
type SyncResult struct {
	BooksAdded   int
	BooksUpdated int
	BooksDeleted int
	Errors       []error
}

// DeviceBook represents a book currently on the device
type DeviceBook struct {
	Filename        string             // e.g., "book123.cbp"
	SidecarFilename string             // e.g., "book123.cbp.xml"
	Metadata        *library.ComicBook // Parsed from sidecar XML (if available)
}

// SyncOperation represents an action to be taken during sync
type SyncOperation struct {
	Type   OperationType
	Book   *library.ComicBook
	Device *DeviceBook
	Reason string
}

// OperationType describes what kind of sync operation to perform
type OperationType int

const (
	OperationAdd                OperationType = iota // Add new book to device
	OperationUpdate                                  // Update existing book (full re-transfer)
	OperationDelete                                  // Delete book from device
	OperationUpdateMetadataOnly                      // Only update sidecar, skip book file
)

// String returns the string representation of an OperationType
func (o OperationType) String() string {
	switch o {
	case OperationAdd:
		return "Add"
	case OperationUpdate:
		return "Update"
	case OperationDelete:
		return "Delete"
	case OperationUpdateMetadataOnly:
		return "UpdateMetadataOnly"
	default:
		return "Unknown"
	}
}

// GetDeviceBooks retrieves the list of books currently on the device
// Returns a map of book ID -> DeviceBook for efficient lookup
func (s *Syncer) GetDeviceBooks() (map[string]*DeviceBook, error) {
	// Get list of all files on device
	fileList, err := s.client.ListFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to list device files: %w", err)
	}

	// Parse file list and identify .cbp and .cbp.xml files
	deviceBooks := make(map[string]*DeviceBook)
	var sidecarFiles []string

	files := strings.Split(fileList, "\n")
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}

		// Check if it's a comic package file (.cbp)
		if strings.HasSuffix(file, ".cbp") && !strings.HasSuffix(file, ".cbp.xml") {
			// Extract book ID from filename (assumes format: {id}.cbp)
			bookID := strings.TrimSuffix(filepath.Base(file), ".cbp")

			if _, exists := deviceBooks[bookID]; !exists {
				deviceBooks[bookID] = &DeviceBook{
					Filename:        file,
					SidecarFilename: file + ".xml",
				}
				// Add sidecar to list of files to fetch
				sidecarFiles = append(sidecarFiles, file+".xml")
			}
		}
	}

	// Fetch all sidecar files at once using ReadMultiFile
	if len(sidecarFiles) > 0 {
		sidecars, err := s.client.ReadMultiFile(sidecarFiles)
		if err != nil {
			// Don't fail the entire operation if we can't read sidecars
			// Just log and continue without metadata
			return deviceBooks, nil
		}

		// Parse each sidecar XML into ComicBook metadata
		for bookID, deviceBook := range deviceBooks {
			sidecarData, ok := sidecars[deviceBook.SidecarFilename]
			if !ok || len(sidecarData) == 0 {
				continue
			}

			var book library.ComicBook
			if err := xml.Unmarshal(sidecarData, &book); err != nil {
				// Skip this book if we can't parse metadata
				continue
			}

			deviceBook.Metadata = &book
			deviceBooks[bookID] = deviceBook
		}
	}

	return deviceBooks, nil
}

// ComputeSyncPlan compares library books against device books and determines
// what operations are needed to synchronize them
func (s *Syncer) ComputeSyncPlan(deviceBooks map[string]*DeviceBook) ([]SyncOperation, error) {
	var operations []SyncOperation

	// Get filtered book list if a filter is set
	var booksToSync []*library.ComicBook
	if s.filterList != nil {
		// Apply smart list filter
		filteredBooks, err := s.library.MatchBooks(s.filterList)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filter list: %w", err)
		}
		booksToSync = filteredBooks
	} else {
		// No filter - sync all books
		booksToSync = make([]*library.ComicBook, len(s.library.Books))
		for i := range s.library.Books {
			booksToSync[i] = &s.library.Books[i]
		}
	}

	// Apply sync settings (filtering, sorting, limiting)
	if s.settings != nil {
		processedBooks, err := ApplySettings(booksToSync, s.settings)
		if err != nil {
			return nil, fmt.Errorf("failed to apply sync settings: %w", err)
		}
		booksToSync = processedBooks
	}

	// Track which library books we've seen
	libraryBookIDs := make(map[string]bool)

	// 1. Check each filtered library book against device
	for _, book := range booksToSync {
		libraryBookIDs[book.ID] = true

		deviceBook, existsOnDevice := deviceBooks[book.ID]

		if !existsOnDevice {
			// Book is in library but not on device -> Add
			operations = append(operations, SyncOperation{
				Type:   OperationAdd,
				Book:   book,
				Reason: "Book not found on device",
			})
		} else {
			// Book exists on device -> Check if update needed
			updateOp, needsUpdate := s.compareBooks(book, deviceBook)
			if needsUpdate {
				operations = append(operations, updateOp)
			}
		}
	}

	// 2. Check for books on device that aren't in library -> Delete
	for bookID, deviceBook := range deviceBooks {
		if !libraryBookIDs[bookID] {
			operations = append(operations, SyncOperation{
				Type:   OperationDelete,
				Device: deviceBook,
				Reason: "Book not in library",
			})
		}
	}

	return operations, nil
}

// compareBooks compares a library book with a device book to determine if an update is needed
// Returns the sync operation and whether an update is needed
func (s *Syncer) compareBooks(libraryBook *library.ComicBook, deviceBook *DeviceBook) (SyncOperation, bool) {
	// If we don't have device metadata, we need to update
	if deviceBook.Metadata == nil {
		return SyncOperation{
			Type:   OperationUpdate,
			Book:   libraryBook,
			Device: deviceBook,
			Reason: "Device metadata unavailable, update required",
		}, true
	}

	// Compare metadata fields to detect changes
	metadataChanged := s.hasMetadataChanged(libraryBook, deviceBook.Metadata)
	pagesChanged := s.hasPagesChanged(libraryBook, deviceBook.Metadata)

	if pagesChanged {
		// Page structure changed - need full re-transfer
		return SyncOperation{
			Type:   OperationUpdate,
			Book:   libraryBook,
			Device: deviceBook,
			Reason: "Page structure changed",
		}, true
	}

	if metadataChanged {
		// Only metadata changed - just update sidecar
		return SyncOperation{
			Type:   OperationUpdateMetadataOnly,
			Book:   libraryBook,
			Device: deviceBook,
			Reason: "Metadata changed",
		}, true
	}

	// No changes needed
	return SyncOperation{}, false
}

// hasMetadataChanged compares metadata fields between library and device books
func (s *Syncer) hasMetadataChanged(library, device *library.ComicBook) bool {
	// Compare key metadata fields that users care about
	return library.Title != device.Title ||
		library.Series != device.Series ||
		library.Number != device.Number ||
		library.Volume != device.Volume ||
		library.Writer != device.Writer ||
		library.Publisher != device.Publisher ||
		library.Year != device.Year ||
		library.Month != device.Month ||
		library.Day != device.Day ||
		library.Rating != device.Rating ||
		library.CurrentPage != device.CurrentPage ||
		library.Summary != device.Summary ||
		library.Notes != device.Notes
}

// hasPagesChanged compares page structure between library and device books
func (s *Syncer) hasPagesChanged(library, device *library.ComicBook) bool {
	// If page counts differ, pages have changed
	if library.PageCount != device.PageCount {
		return true
	}

	// If page array lengths differ, pages have changed
	if len(library.Pages) != len(device.Pages) {
		return true
	}

	// Compare each page's type and image index
	for i := range library.Pages {
		if library.Pages[i].Image != device.Pages[i].Image ||
			library.Pages[i].Type != device.Pages[i].Type {
			return true
		}
	}

	return false
}
