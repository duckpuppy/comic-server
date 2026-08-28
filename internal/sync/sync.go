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
	backend      library.Backend                // Backend for library operations (replaces library + libraryCache)
	filterList   *library.ComicListItem         // Optional single smart list to filter books (deprecated, use filterLists)
	filterLists  []*library.ComicListItem       // Optional multiple smart lists to filter books (union of all lists)
	listSettings map[string]*SharedListSettings // ListID -> per-list settings override; see SetFilterListsWithSettings
	settings     *SharedListSettings            // Default sync settings - applied to a list with no per-list override in listSettings, or globally when filterLists isn't used

	// resolvePath translates a book's raw library FilePath into a path
	// actually readable on this filesystem, before any direct file read -
	// see SetPathResolver. Defaults to the identity function: most
	// deployments run on the same OS/filesystem that wrote the library
	// XML, so no translation is needed.
	resolvePath func(string) string

	// onStatusDetail, if set, is called with a human-readable status
	// string whenever the sync is in a state that isn't visible progress
	// but also isn't a hang - e.g. retrying a device whose TCP listener
	// isn't up yet (comic-server-134). See SetStatusDetailCallback.
	onStatusDetail func(detail string)

	// onProgress, if set, is called after every operation in the forward-
	// sync loop (PerformSync's Step 6, in session.go) - success or
	// failure - with the running percent complete and add/update/delete/
	// error counts so far. Without this, a sync's visible progress only
	// ever moved at start (0%) and end (100%): syncstate.Manager.
	// UpdateProgress existed but nothing ever called it during the loop
	// itself (comic-server-p0x). See SetProgressCallback.
	onProgress func(percent, total, added, updated, deleted, errorCount int)

	// connRefusedGracePeriod is how long GetDeviceBooks's sidecar-read
	// circuit breaker keeps retrying past its normal failure threshold
	// when every recent failure is specifically "connection refused" -
	// see the comment at its use site. A field (not a plain const) so
	// tests can shrink it instead of running real-time for tens of
	// seconds.
	connRefusedGracePeriod time.Duration
}

// defaultConnRefusedGracePeriod is connRefusedGracePeriod's production
// value.
const defaultConnRefusedGracePeriod = 25 * time.Second

// NewSyncer creates a new sync orchestrator
func NewSyncer(client Client, backend library.Backend) *Syncer {
	return &Syncer{
		client:                 client,
		backend:                backend,
		filterList:             nil,
		settings:               DefaultSettings(), // Use default settings
		resolvePath:            func(p string) string { return p },
		onStatusDetail:         func(string) {},
		onProgress:             func(int, int, int, int, int, int) {},
		connRefusedGracePeriod: defaultConnRefusedGracePeriod,
	}
}

// SetStatusDetailCallback registers a callback invoked with a
// human-readable status string during a sync stretch that's neither
// failing nor visibly progressing (currently: while retrying a device
// that's refusing connections early in a sync, within its startup grace
// period). Callers use this to surface that detail somewhere a user can
// see it - e.g. syncstate.Manager.SetDetail - without this package needing
// to know anything about syncstate. Passing nil restores the default
// no-op.
func (s *Syncer) SetStatusDetailCallback(fn func(detail string)) {
	if fn == nil {
		fn = func(string) {}
	}
	s.onStatusDetail = fn
}

// SetProgressCallback registers a callback invoked after every forward-sync
// operation (success or failure) with the running percent complete and
// add/update/delete/error counts so far - see onProgress's doc comment.
// Callers use this to feed syncstate.Manager.UpdateProgress without this
// package needing to know anything about syncstate. Passing nil restores
// the default no-op.
func (s *Syncer) SetProgressCallback(fn func(percent, total, added, updated, deleted, errorCount int)) {
	if fn == nil {
		fn = func(int, int, int, int, int, int) {}
	}
	s.onProgress = fn
}

// SetPathResolver overrides how a book's raw library FilePath is translated
// into a path this process can actually read, before every direct file
// access (the real comic file transfer, and free-space/size estimates).
// Needed whenever the library XML was authored on a different OS/mount than
// this comic-server process runs on (e.g. a Windows-authored library served
// from a Linux container) - see config.Config.ResolveLibraryFilePath, the
// intended source of the function passed here. Without this, every direct
// file read fails with "no such file or directory" despite the book
// existing - see comic-server-4n9, the same class of bug as
// comic-server-ivq (cover extraction) had before it was fixed.
func (s *Syncer) SetPathResolver(resolve func(string) string) {
	if resolve == nil {
		resolve = func(p string) string { return p }
	}
	s.resolvePath = resolve
}

// SetFilterList sets a list to filter which books get synced - any list
// type with real book membership works (smart list, ID list, reading
// list; see GetBooksForList), not just smart lists. Folders are rejected
// since they group other lists rather than containing books themselves.
// Pass nil to sync all books.
// Deprecated: Use SetFilterLists for multi-list support
func (s *Syncer) SetFilterList(list *library.ComicListItem) error {
	if list != nil && strings.Contains(list.Type, "Folder") {
		return fmt.Errorf("list %q is a folder, not something books can sync from (type: %s)", list.Name, list.Type)
	}
	s.filterList = list
	s.filterLists = nil // Clear multi-list if single list is set
	return nil
}

// SetFilterLists sets multiple lists to filter which books get synced -
// same list-type support as SetFilterList. Books matching ANY list will
// be synced (union of all lists). Pass nil or empty slice to sync all
// books.
func (s *Syncer) SetFilterLists(lists []*library.ComicListItem) error {
	for _, list := range lists {
		if list != nil && strings.Contains(list.Type, "Folder") {
			return fmt.Errorf("list %q is a folder, not something books can sync from (type: %s)", list.Name, list.Type)
		}
	}
	s.filterLists = lists
	s.filterList = nil // Clear single list if multi-list is set
	s.listSettings = nil
	return nil
}

// FilterListEntry pairs one filter list with its own sync settings, for
// SetFilterListsWithSettings.
type FilterListEntry struct {
	List     *library.ComicListItem
	Settings *SharedListSettings // nil = use the Syncer's default settings (see SetSettings)
}

// SetFilterListsWithSettings is SetFilterLists, but lets each list carry
// its own SharedListSettings (only-unread, limit, sort, etc.) instead of
// every list sharing one global settings object. Each list's own settings
// (filtering, sorting, limiting) are applied to that list's matched books
// before the union is taken, so e.g. one list can be "unread only" while
// another isn't. A list whose Settings is nil falls back to the Syncer's
// default settings (SetSettings) - same behavior as before this method
// existed. See comic-server-3oq: before this, every list silently shared
// one settings object the moment there was more than one of them,
// discarding per-list only-unread/limit/sort configuration outright.
func (s *Syncer) SetFilterListsWithSettings(entries []FilterListEntry) error {
	lists := make([]*library.ComicListItem, 0, len(entries))
	listSettings := make(map[string]*SharedListSettings, len(entries))
	for _, entry := range entries {
		if entry.List != nil && strings.Contains(entry.List.Type, "Folder") {
			return fmt.Errorf("list %q is a folder, not something books can sync from (type: %s)", entry.List.Name, entry.List.Type)
		}
		lists = append(lists, entry.List)
		if entry.Settings != nil && entry.List != nil {
			listSettings[entry.List.ID] = entry.Settings
		}
	}
	s.filterLists = lists
	s.filterList = nil
	s.listSettings = listSettings
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
			// Circuit breaker for a device that's gone completely
			// unreachable (not just slow): a run of consecutive failures
			// this long means every retry from here on is doomed too, so
			// bail out of the whole sync attempt now instead of grinding
			// through the rest of potentially thousands of sidecar files,
			// each with its own 3 retries, only to then fail every write
			// operation the same way. Also fixes a real-world lockout:
			// while a sync is running the device can't start another one
			// (StartSync rejects it), so a hung, doomed sync here directly
			// blocked the user from retrying for as long as it took to
			// exhaust every remaining file - found live 2026-08-27.
			//
			// connRefusedGracePeriod extends that circuit breaker
			// specifically for "connection refused" failures early in a
			// sync: the ComicRack Android app broadcasts its ':Sync'
			// intent before its own TCP listener is actually up (found
			// live 2026-08-27/28, comic-server-134), so a burst of
			// connection-refused errors in the first few seconds of a
			// sync usually means "give it a few more seconds," not
			// "genuinely gone." Any other error shape (timeout, reset,
			// unreachable) still hits the tight maxConsecutiveFailures
			// cutoff - those don't carry the same "about to come up"
			// implication connection-refused does. The grace period is
			// timed from firstConnRefusedAt, not sync start, so a device
			// that answers fine for a while and only later goes
			// perma-refused still gets the same fast bailout everything
			// else gets.
			const maxConsecutiveFailures = 10
			consecutiveFailures := 0
			var firstConnRefusedAt time.Time
			for _, sidecarFile := range sidecarFiles {
				if consecutiveFailures >= maxConsecutiveFailures {
					inGracePeriod := !firstConnRefusedAt.IsZero() && time.Since(firstConnRefusedAt) < s.connRefusedGracePeriod
					if !inGracePeriod {
						return nil, fmt.Errorf("device unreachable: %d consecutive sidecar reads failed", consecutiveFailures)
					}
					log.Debug().
						Int("consecutive_failures", consecutiveFailures).
						Dur("elapsed_since_first_refused", time.Since(firstConnRefusedAt)).
						Msg("Device still refusing connections, within startup grace period - continuing to retry")
					s.onStatusDetail("device not responding yet, retrying")
				}

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
					consecutiveFailures++
					if protocol.IsConnectionRefused(err) {
						if firstConnRefusedAt.IsZero() {
							firstConnRefusedAt = time.Now()
						}
					} else {
						// Not a "still starting up" error - don't let an
						// earlier streak of refusals extend the grace
						// period for a different kind of failure.
						firstConnRefusedAt = time.Time{}
					}
					log.Warn().
						Err(err).
						Str("sidecar", sidecarFile).
						Int("attempts", maxRetries).
						Msg("Failed to read individual sidecar file after all retries")
					continue
				}
				consecutiveFailures = 0
				firstConnRefusedAt = time.Time{}
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

// booksForFilterList returns the books matched by one filter list, with
// that list's own settings applied (or the Syncer's default settings, for
// a list with no override) - the per-list unit both computeUnionOfLists
// (for the sync plan) and the sync_information.xml writer in session.go
// (for reporting each list's own membership to the device) build on, so
// both agree on what's actually "in" a given list.
func (s *Syncer) booksForFilterList(list *library.ComicListItem) ([]*library.ComicBook, error) {
	if list == nil {
		return nil, nil
	}

	// GetBooksForList (not MatchBooks) so this works for every list type a
	// device can be assigned: smart lists, ID lists, and reading lists
	// (same class of fix as comic-server-vwl's Komga-target fix).
	// MatchBooks only evaluates matcher rules, which an ID/reading list
	// doesn't have.
	matchedBooks, err := s.backend.GetBooksForList(list)
	if err != nil {
		// Matches computeUnionOfLists' pre-refactor behavior: log-and-skip
		// rather than fail the whole sync over one bad list.
		return nil, nil
	}

	settings := s.settings
	if override, ok := s.listSettings[list.ID]; ok {
		settings = override
	}
	if settings == nil {
		return matchedBooks, nil
	}
	matchedBooks, err = ApplySettingsWithResolver(matchedBooks, settings, s.resolvePath)
	if err != nil {
		return nil, fmt.Errorf("failed to apply settings for list %q: %w", list.Name, err)
	}
	return matchedBooks, nil
}

// computeUnionOfLists computes the union of all filter lists, applying
// each list's own settings (see SetFilterListsWithSettings) - or the
// Syncer's default settings, for a list with no override - to that list's
// matched books BEFORE the union is taken. This is what makes e.g.
// "unread only" on one list not bleed into (or get discarded by) another
// list synced alongside it; see comic-server-3oq.
// Returns all books that survive ANY of the filter lists' own settings
// (no duplicates).
func (s *Syncer) computeUnionOfLists() ([]*library.ComicBook, error) {
	bookMap := make(map[string]*library.ComicBook)

	for _, list := range s.filterLists {
		matchedBooks, err := s.booksForFilterList(list)
		if err != nil {
			return nil, err
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

	return result, nil
}

// ComputeSyncPlan compares library books against device books and determines
// what operations are needed to synchronize them
func (s *Syncer) ComputeSyncPlan(deviceBooks map[string]*DeviceBook) ([]SyncOperation, error) {
	var operations []SyncOperation

	// Get filtered book list if a filter is set
	var booksToSync []*library.ComicBook
	if len(s.filterLists) > 0 {
		// Apply multiple smart list filters (union of all lists) - each
		// list's own settings (or the default, for a list with no
		// override) are applied per-list inside computeUnionOfLists, so
		// skip the global settings pass below for this branch.
		var err error
		booksToSync, err = s.computeUnionOfLists()
		if err != nil {
			return nil, err
		}
	} else if s.filterList != nil {
		// Apply single list filter (backward compatibility) - GetBooksForList,
		// see computeUnionOfLists for why.
		filteredBooks, err := s.backend.GetBooksForList(s.filterList)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filter list: %w", err)
		}
		booksToSync = filteredBooks

		// Apply sync settings (filtering, sorting, limiting)
		if s.settings != nil {
			processedBooks, err := ApplySettingsWithResolver(booksToSync, s.settings, s.resolvePath)
			if err != nil {
				return nil, fmt.Errorf("failed to apply sync settings: %w", err)
			}
			booksToSync = processedBooks
		}
	} else {
		// No filter - sync all books
		allBooks, err := s.backend.GetAllBooks()
		if err != nil {
			return nil, fmt.Errorf("failed to get all books: %w", err)
		}
		booksToSync = make([]*library.ComicBook, len(allBooks))
		for i := range allBooks {
			booksToSync[i] = &allBooks[i]
		}

		// Apply sync settings (filtering, sorting, limiting)
		if s.settings != nil {
			processedBooks, err := ApplySettingsWithResolver(booksToSync, s.settings, s.resolvePath)
			if err != nil {
				return nil, fmt.Errorf("failed to apply sync settings: %w", err)
			}
			booksToSync = processedBooks
		}
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
