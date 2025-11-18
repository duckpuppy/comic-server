package sync

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
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
	SendStart(message ...string) error
	SendCompleted() error
	SendProgressUpdate(percent int) error
	GetFreeSpace() (int64, error)
	CheckAbort() (bool, error)
}

// Syncer orchestrates synchronization between the library and a device
type Syncer struct {
	client       Client
	library      *library.ComicLibrary
	libraryCache *library.LibraryCache    // Library cache for batched saves (optional)
	libraryPath  string                   // Path to library XML file for saving updates (deprecated when using cache)
	filterList   *library.ComicListItem   // Optional single smart list to filter books (deprecated, use filterLists)
	filterLists  []*library.ComicListItem // Optional multiple smart lists to filter books (union of all lists)
	settings     *SharedListSettings      // Sync settings to apply (filtering, sorting, limiting)
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

// SetLibraryPath sets the path to the library XML file for saving updates
// Deprecated: Use SetLibraryCache for better performance
func (s *Syncer) SetLibraryPath(path string) {
	s.libraryPath = path
}

// SetLibraryCache sets the library cache for batched saves
// This should be used instead of SetLibraryPath for better performance
func (s *Syncer) SetLibraryCache(cache *library.LibraryCache) {
	s.libraryCache = cache
}

// SetFilterList sets a smart list to filter which books get synced
// Pass nil to sync all books
// Deprecated: Use SetFilterLists for multi-list support
func (s *Syncer) SetFilterList(list *library.ComicListItem) error {
	if list != nil {
		// Validate it's a smart list
		if !strings.Contains(list.Type, "SmartList") {
			return fmt.Errorf("list %q is not a smart list (type: %s)", list.Name, list.Type)
		}
	}
	s.filterList = list
	s.filterLists = nil // Clear multi-list if single list is set
	return nil
}

// SetFilterLists sets multiple smart lists to filter which books get synced
// Books matching ANY list will be synced (union of all lists)
// Pass nil or empty slice to sync all books
func (s *Syncer) SetFilterLists(lists []*library.ComicListItem) error {
	// Validate all lists are smart lists
	for _, list := range lists {
		if list != nil && !strings.Contains(list.Type, "SmartList") {
			return fmt.Errorf("list %q is not a smart list (type: %s)", list.Name, list.Type)
		}
	}
	s.filterLists = lists
	s.filterList = nil // Clear single list if multi-list is set
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
		log.Debug().Int("sidecar_count", len(sidecarFiles)).Msg("Reading sidecar files")
		sidecars, err := s.client.ReadMultiFile(sidecarFiles)
		if err != nil {
			// ReadMultiFile failed - try reading sidecars individually as fallback
			log.Warn().
				Err(err).
				Int("sidecar_count", len(sidecarFiles)).
				Msg("Batch read failed, falling back to individual file reads")

			sidecars = make(map[string][]byte)
			successCount := 0
			for _, sidecarFile := range sidecarFiles {
				// Try to read the file with retries
				var data []byte
				var err error
				maxRetries := 3
				for attempt := 0; attempt < maxRetries; attempt++ {
					if attempt > 0 {
						// Add delay between retries (exponential backoff: 100ms, 200ms, 400ms)
						delay := time.Duration(100<<uint(attempt-1)) * time.Millisecond
						log.Debug().
							Str("sidecar", sidecarFile).
							Int("attempt", attempt+1).
							Dur("delay", delay).
							Msg("Retrying sidecar read after delay")
						time.Sleep(delay)
					}

					data, err = s.client.ReadFile(sidecarFile)
					if err == nil {
						break // Success!
					}

					if attempt < maxRetries-1 {
						log.Debug().
							Err(err).
							Str("sidecar", sidecarFile).
							Int("attempt", attempt+1).
							Int("max_retries", maxRetries).
							Msg("Sidecar read failed, will retry")
					}
				}

				if err != nil {
					log.Warn().
						Err(err).
						Str("sidecar", sidecarFile).
						Int("attempts", maxRetries).
						Msg("Failed to read individual sidecar file after all retries")
					continue
				}
				sidecars[sidecarFile] = data
				successCount++

				// Small delay between successful reads to avoid overwhelming device
				time.Sleep(10 * time.Millisecond)
			}
			log.Info().
				Int("success", successCount).
				Int("total", len(sidecarFiles)).
				Msg("Individual sidecar reads completed")
		} else {
			log.Debug().Int("sidecars_read", len(sidecars)).Msg("Successfully read sidecar files via batch read")
		}

		// Parse each sidecar XML into ComicBook metadata
		// Build a new map with correct book IDs from sidecars
		correctDeviceBooks := make(map[string]*DeviceBook)
		for _, deviceBook := range deviceBooks {
			sidecarData, ok := sidecars[deviceBook.SidecarFilename]
			if !ok || len(sidecarData) == 0 {
				// No sidecar - use filename as key (shouldn't happen in normal operation)
				filenameKey := strings.TrimSuffix(filepath.Base(deviceBook.Filename), ".cbp")
				log.Warn().
					Str("filename", deviceBook.Filename).
					Str("filename_key", filenameKey).
					Msg("No sidecar found for device book, using filename as key")
				correctDeviceBooks[filenameKey] = deviceBook
				continue
			}

			var book library.ComicBook
			if err := xml.Unmarshal(sidecarData, &book); err != nil {
				// Can't parse sidecar - use filename as key
				filenameKey := strings.TrimSuffix(filepath.Base(deviceBook.Filename), ".cbp")
				sidecarPreview := string(sidecarData)
				if len(sidecarPreview) > 500 {
					sidecarPreview = sidecarPreview[:500]
				}
				log.Error().
					Err(err).
					Str("filename", deviceBook.Filename).
					Str("filename_key", filenameKey).
					Str("sidecar_preview", sidecarPreview).
					Msg("Failed to parse sidecar XML")
				correctDeviceBooks[filenameKey] = deviceBook
				continue
			}

			// Use the actual book ID (GUID) from sidecar as the map key
			log.Debug().
				Str("filename", deviceBook.Filename).
				Str("book_id", book.ID).
				Str("title", book.Title).
				Msg("Successfully parsed sidecar XML")
			deviceBook.Metadata = &book
			correctDeviceBooks[book.ID] = deviceBook
		}
		return correctDeviceBooks, nil
	}

	return deviceBooks, nil
}

// computeUnionOfLists computes the union of all filter lists
// Returns all books that match ANY of the filter lists (no duplicates)
func (s *Syncer) computeUnionOfLists() []*library.ComicBook {
	bookMap := make(map[string]*library.ComicBook)

	for _, list := range s.filterLists {
		if list == nil {
			continue
		}

		// Get books matching this list
		matchedBooks, err := s.library.MatchBooks(list)
		if err != nil {
			// Log error but continue with other lists
			continue
		}

		// Add to union (map automatically deduplicates by book ID)
		for _, book := range matchedBooks {
			bookMap[book.ID] = book
		}
	}

	// Convert map to slice
	result := make([]*library.ComicBook, 0, len(bookMap))
	for _, book := range bookMap {
		result = append(result, book)
	}

	return result
}

// ComputeSyncPlan compares library books against device books and determines
// what operations are needed to synchronize them
func (s *Syncer) ComputeSyncPlan(deviceBooks map[string]*DeviceBook) ([]SyncOperation, error) {
	var operations []SyncOperation

	// Get filtered book list if a filter is set
	var booksToSync []*library.ComicBook
	if len(s.filterLists) > 0 {
		// Apply multiple smart list filters (union of all lists)
		booksToSync = s.computeUnionOfLists()
	} else if s.filterList != nil {
		// Apply single smart list filter (backward compatibility)
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

	// DEBUG: Print library book IDs
	log.Debug().Int("count", len(booksToSync)).Msg("DEBUG: Library books to sync:")
	for i, book := range booksToSync {
		if i < 5 { // Only print first 5 to avoid spam
			log.Debug().
				Str("id", book.ID).
				Str("title", book.Title).
				Str("filepath", book.FilePath).
				Msgf("  Library book %d", i+1)
		}
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
		// Debug: Log which fields changed
		log.Debug().
			Str("book_id", libraryBook.ID).
			Str("title", libraryBook.Title).
			Bool("title_changed", libraryBook.Title != deviceBook.Metadata.Title).
			Bool("series_changed", libraryBook.Series != deviceBook.Metadata.Series).
			Bool("number_changed", libraryBook.Number != deviceBook.Metadata.Number).
			Bool("volume_changed", libraryBook.Volume != deviceBook.Metadata.Volume).
			Bool("writer_changed", libraryBook.Writer != deviceBook.Metadata.Writer).
			Bool("publisher_changed", libraryBook.Publisher != deviceBook.Metadata.Publisher).
			Bool("year_changed", libraryBook.Year != deviceBook.Metadata.Year).
			Bool("month_changed", libraryBook.Month != deviceBook.Metadata.Month).
			Bool("day_changed", libraryBook.Day != deviceBook.Metadata.Day).
			Bool("rating_changed", libraryBook.Rating != deviceBook.Metadata.Rating).
			Bool("current_page_changed", libraryBook.CurrentPage != deviceBook.Metadata.CurrentPage).
			Bool("summary_changed", libraryBook.Summary != deviceBook.Metadata.Summary).
			Bool("notes_changed", libraryBook.Notes != deviceBook.Metadata.Notes).
			Msg("Metadata changed - which fields differ")

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
	// PageCount is the authoritative field - compare this, not len(Pages)
	// Note: ComicRack library XML only stores Page entries for pages with metadata
	// (cover type, bookmarks, etc.), not all pages. The PageCount field contains
	// the actual total page count from scanning the comic file.
	if library.PageCount != device.PageCount {
		log.Debug().
			Str("book_id", library.ID).
			Str("title", library.Title).
			Int("library_page_count", library.PageCount).
			Int("device_page_count", device.PageCount).
			Msg("Page count differs")
		return true
	}

	// Only compare individual page metadata if BOTH books have the same number
	// of Page entries. If they differ, it just means one has full page metadata
	// and the other has sparse metadata - not a real change.
	if len(library.Pages) == len(device.Pages) && len(library.Pages) > 0 {
		// Compare each page's type and image index
		for i := range library.Pages {
			if library.Pages[i].Image != device.Pages[i].Image ||
				library.Pages[i].Type != device.Pages[i].Type {
				log.Debug().
					Str("book_id", library.ID).
					Str("title", library.Title).
					Int("page_index", i).
					Int("library_image", library.Pages[i].Image).
					Int("device_image", device.Pages[i].Image).
					Str("library_type", string(library.Pages[i].Type)).
					Str("device_type", string(device.Pages[i].Type)).
					Msg("Page metadata differs")
				return true
			}
		}
	}

	// Pages are the same (PageCount matches and either page metadata matches
	// or one/both books have sparse page metadata)
	return false
}
