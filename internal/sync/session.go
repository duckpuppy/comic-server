package sync

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// PerformSync executes a full synchronization session with a device
// Follows the sync flow from WIRELESS_SYNC_PROTOCOL.md:864-912
func (s *Syncer) PerformSync() (*SyncResult, error) {
	result := &SyncResult{
		Errors: make([]error, 0),
	}

	// Step 1: CommandStart - Begin sync session
	log.Info().Msg("Starting synchronization session")
	if err := s.client.SendStart(); err != nil {
		return result, fmt.Errorf("failed to start sync: %w", err)
	}

	// Step 2: CommandInfo - Validate device
	log.Info().Msg("Validating device")
	deviceInfo, err := s.client.GetDeviceInfo()
	if err != nil {
		return result, fmt.Errorf("failed to get device info: %w", err)
	}
	log.Info().
		Bool("licensed", deviceInfo.Licensed).
		Int("version", deviceInfo.VersionCode).
		Msg("Device validated")

	// Step 3: CommandFreeSpace - Check available storage
	log.Info().Msg("Checking device storage")
	freeSpace, err := s.client.GetFreeSpace()
	if err != nil {
		return result, fmt.Errorf("failed to get free space: %w", err)
	}
	log.Info().
		Int64("bytes", freeSpace).
		Float64("mb", float64(freeSpace)/(1024*1024)).
		Msg("Device free space")

	// Step 4: Get current device state
	log.Info().Msg("Retrieving device book list")
	deviceBooks, err := s.GetDeviceBooks()
	if err != nil {
		return result, fmt.Errorf("failed to get device books: %w", err)
	}
	log.Info().Int("count", len(deviceBooks)).Msg("Found books on device")

	// DEBUG: Print device book IDs
	log.Debug().Msg("Device book IDs:")
	for bookID, deviceBook := range deviceBooks {
		title := "(no metadata)"
		if deviceBook.Metadata != nil {
			title = deviceBook.Metadata.Title
		}
		log.Debug().
			Str("id", bookID).
			Str("file", deviceBook.Filename).
			Str("title", title).
			Msg("Device book")
	}

	// Step 4.5: Update library with reading state from device (reverse sync)
	// This MUST happen BEFORE forward sync to capture device changes before overwriting them
	log.Info().Msg("Updating library with reading state from device")
	if err := s.updateLibraryReadingState(deviceBooks); err != nil {
		// Don't fail sync if reading state update fails
		log.Warn().Err(err).Msg("Failed to update library reading state")
	}

	// Step 5: Compute sync plan
	log.Info().Msg("Computing sync plan")
	operations, err := s.ComputeSyncPlan(deviceBooks)
	if err != nil {
		return result, fmt.Errorf("failed to compute sync plan: %w", err)
	}
	log.Info().Int("operations", len(operations)).Msg("Sync plan computed")

	// Count operations by type for logging
	addCount := 0
	updateCount := 0
	deleteCount := 0
	metadataOnlyCount := 0
	for _, op := range operations {
		switch op.Type {
		case OperationAdd:
			addCount++
		case OperationUpdate:
			updateCount++
		case OperationDelete:
			deleteCount++
		case OperationUpdateMetadataOnly:
			metadataOnlyCount++
		}
	}
	log.Info().
		Int("add", addCount).
		Int("update", updateCount).
		Int("delete", deleteCount).
		Int("metadata_only", metadataOnlyCount).
		Msg("Operation breakdown")

	// Validate storage space
	requiredSpace, err := s.calculateRequiredSpace(operations)
	if err != nil {
		return result, fmt.Errorf("failed to calculate required space: %w", err)
	}
	log.Info().
		Int64("bytes", requiredSpace).
		Float64("mb", float64(requiredSpace)/(1024*1024)).
		Msg("Required space")

	// Add 10% buffer for overhead
	requiredSpaceWithBuffer := int64(float64(requiredSpace) * 1.1)
	if requiredSpaceWithBuffer > freeSpace {
		return result, fmt.Errorf("insufficient storage: need %.2f MB, have %.2f MB",
			float64(requiredSpaceWithBuffer)/(1024*1024),
			float64(freeSpace)/(1024*1024))
	}

	// Sort operations for safer execution order:
	// 1. ADD - New books (safest, nothing to lose if sync fails)
	// 2. UPDATE/UpdateMetadataOnly - Update existing books (delete+add atomically)
	// 3. DELETE - Remove books not in library (only at end)
	//
	// This ordering ensures that if sync fails partway through, the device
	// retains as much content as possible. UPDATE operations are handled
	// atomically (delete old, add new immediately), so if an update fails
	// after deletion, the next sync will detect it's missing and re-add it.
	operations = sortOperationsByType(operations)
	log.Debug().Msg("Operations sorted for safe execution order")

	// Track filenames written during this sync to avoid deleting them
	// This prevents the case where a book fails sidecar read (loses ID mapping),
	// gets re-added with correct ID, then incorrectly deleted as an "orphan"
	writtenFiles := make(map[string]bool)

	// Step 6: Execute sync operations
	totalOps := len(operations)
	for i, op := range operations {
		// Check for abort before every operation - not every 10th - so a
		// user-initiated abort takes effect as soon as whatever operation
		// is currently in flight finishes, rather than after up to 9 more
		// run first (comic-server-akt). This trades one extra round trip
		// to the device per operation for that responsiveness.
		//
		// A failed check (not just aborted=true) is treated as a stop
		// signal too: a device that can't even answer CheckAbort almost
		// certainly can't take a WriteFile either, and continuing to
		// grind through the remaining queue would just be a long series
		// of doomed, individually-timing-out operations against a device
		// that's already gone.
		if aborted, err := s.client.CheckAbort(); err != nil {
			return result, fmt.Errorf("sync stopped: device unresponsive to abort check: %w", err)
		} else if aborted {
			return result, fmt.Errorf("sync aborted by user")
		}

		// Execute operation
		if err := s.executeOperation(op, writtenFiles); err != nil {
			errMsg := fmt.Errorf("operation %d/%d failed (%s for %s): %w",
				i+1, totalOps, op.Type, getTitleForOp(op), err)
			result.Errors = append(result.Errors, errMsg)
			log.Error().Err(errMsg).Msg("Operation failed")
			continue
		}

		// Update counters
		switch op.Type {
		case OperationAdd:
			result.BooksAdded++
		case OperationUpdate, OperationUpdateMetadataOnly:
			result.BooksUpdated++
		case OperationDelete:
			result.BooksDeleted++
		}

		// Update progress
		percent := ((i + 1) * 100) / totalOps
		if err := s.client.SendProgressUpdate(percent); err != nil {
			log.Warn().Err(err).Msg("Failed to send progress update")
		}

		log.Info().
			Int("current", i+1).
			Int("total", totalOps).
			Str("operation", op.Type.String()).
			Str("title", getTitleForOp(op)).
			Msg("Operation completed")
	}

	// Step 8: Write sync_information.xml (reading lists)
	log.Info().Msg("Writing reading lists")
	if err := s.writeSyncInformation(); err != nil {
		// Don't fail sync if reading list write fails
		log.Warn().Err(err).Msg("Failed to write reading lists")
	}

	// Step 9: Final progress update to 100%
	if err := s.client.SendProgressUpdate(100); err != nil {
		log.Warn().Err(err).Msg("Failed to send final progress")
	}

	// Step 10: Touch marker file (comicrack.ini) to trigger device refresh
	// This is critical - updating the marker file timestamp triggers the Android
	// app to detect changes and refresh the library display
	log.Info().Msg("Updating marker file timestamp")
	if err := s.touchMarkerFile(); err != nil {
		log.Warn().Err(err).Msg("Failed to touch marker file")
	}

	// Step 11: CommandCompleted - Signal sync completion
	log.Info().Msg("Completing synchronization")
	if err := s.client.SendCompleted(); err != nil {
		return result, fmt.Errorf("failed to send completion: %w", err)
	}

	log.Info().
		Int("added", result.BooksAdded).
		Int("updated", result.BooksUpdated).
		Int("deleted", result.BooksDeleted).
		Int("errors", len(result.Errors)).
		Msg("Sync complete")

	return result, nil
}

// executeOperation performs a single sync operation
func (s *Syncer) executeOperation(op SyncOperation, writtenFiles map[string]bool) error {
	switch op.Type {
	case OperationAdd:
		filename, err := s.addBook(op.Book)
		if err == nil && filename != "" {
			writtenFiles[filename] = true
		}
		return err
	case OperationUpdate:
		filename, err := s.updateBook(op.Book, op.Device)
		if err == nil && filename != "" {
			writtenFiles[filename] = true
		}
		return err
	case OperationUpdateMetadataOnly:
		return s.updateMetadataOnly(op.Book, op.Device)
	case OperationDelete:
		return s.deleteBook(op.Device, writtenFiles)
	default:
		return fmt.Errorf("unknown operation type: %v", op.Type)
	}
}

// addBook adds a new book to the device
// Returns the filename that was written (for tracking)
func (s *Syncer) addBook(book *library.ComicBook) (string, error) {
	// 1. Determine target filename on device
	// Use actual filename from library, not GUID
	baseFilename := filepath.Base(book.FilePath)
	// Change extension to .cbp for device storage
	filename := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename)) + ".cbp"

	// 2. Check if file exists (shouldn't, but be safe)
	exists, err := s.client.FileExists(filename)
	if err != nil {
		return "", fmt.Errorf("failed to check if file exists: %w", err)
	}
	if exists {
		// File exists but wasn't in our book list - delete it first
		if err := s.client.DeleteFile(filename); err != nil {
			return "", fmt.Errorf("failed to delete existing file: %w", err)
		}
	}

	// 2. Write comic book file (.cbp)
	bookData, err := s.readComicFile(book)
	if err != nil {
		return "", fmt.Errorf("failed to read comic file: %w", err)
	}
	if err := s.client.WriteFile(filename, bookData); err != nil {
		return "", fmt.Errorf("failed to write book file: %w", err)
	}

	// 3. Write sidecar metadata (.cbp.xml)
	sidecarData, err := s.generateSidecar(book)
	if err != nil {
		return "", fmt.Errorf("failed to generate sidecar: %w", err)
	}
	sidecarFilename := filename + ".xml"
	log.Debug().
		Str("filename", filename).
		Str("sidecar", sidecarFilename).
		Str("book_id", book.ID).
		Str("title", book.Title).
		Int("sidecar_bytes", len(sidecarData)).
		Msg("Writing sidecar file")
	if err := s.client.WriteFile(sidecarFilename, sidecarData); err != nil {
		return "", fmt.Errorf("failed to write sidecar: %w", err)
	}

	return filename, nil
}

// updateBook updates an existing book on the device (full re-transfer)
// Returns the filename that was written (for tracking)
func (s *Syncer) updateBook(book *library.ComicBook, device *DeviceBook) (string, error) {
	// Delete old files (don't pass writtenFiles since this is the same operation)
	writtenFiles := make(map[string]bool) // Empty map for this sub-operation
	if err := s.deleteBook(device, writtenFiles); err != nil {
		return "", fmt.Errorf("failed to delete old book: %w", err)
	}

	// Add new version
	return s.addBook(book)
}

// updateMetadataOnly updates only the sidecar metadata, not the book file
func (s *Syncer) updateMetadataOnly(book *library.ComicBook, device *DeviceBook) error {
	// Generate and write new sidecar
	sidecarData, err := s.generateSidecar(book)
	if err != nil {
		return fmt.Errorf("failed to generate sidecar: %w", err)
	}

	if err := s.client.WriteFile(device.SidecarFilename, sidecarData); err != nil {
		return fmt.Errorf("failed to write sidecar: %w", err)
	}

	return nil
}

// deleteBook removes a book from the device
func (s *Syncer) deleteBook(device *DeviceBook, writtenFiles map[string]bool) error {
	// Check if this file was written during this sync session
	// If so, skip deletion to avoid deleting a file we just added
	// This happens when a sidecar read fails (loses ID mapping), causing the same
	// filename to appear as both "add" (correct ID) and "delete" (orphaned entry)
	if writtenFiles[device.Filename] {
		log.Info().
			Str("filename", device.Filename).
			Msg("Skipping delete - file was written during this sync")
		return nil
	}

	// Send status message to device
	// Extract filename without extension for display
	displayName := strings.TrimSuffix(device.Filename, filepath.Ext(device.Filename))
	statusMsg := fmt.Sprintf("Removing '%s' from device", displayName)
	if err := s.client.SendStart(statusMsg); err != nil {
		// Don't fail delete if status message fails
		log.Warn().Err(err).Str("filename", displayName).Msg("Failed to send delete status")
	}

	// Delete comic file
	if err := s.client.DeleteFile(device.Filename); err != nil {
		return fmt.Errorf("failed to delete book file: %w", err)
	}

	// Delete sidecar
	if err := s.client.DeleteFile(device.SidecarFilename); err != nil {
		// Don't fail if sidecar doesn't exist
		return nil
	}

	return nil
}

// generateSidecar creates sidecar XML from a ComicBook
func (s *Syncer) generateSidecar(book *library.ComicBook) ([]byte, error) {
	// Marshal the ComicBook to XML
	data, err := xml.MarshalIndent(book, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal book metadata: %w", err)
	}

	// Add XML declaration
	xmlData := []byte(xml.Header + string(data))

	// Debug: Log XML preview
	preview := string(xmlData)
	if len(preview) > 500 {
		preview = preview[:500]
	}
	log.Debug().
		Str("book_id", book.ID).
		Str("xml_preview", preview).
		Msg("Generated sidecar XML")

	return xmlData, nil
}

// SyncInformation represents the sync_information.xml structure
type SyncInformation struct {
	XMLName xml.Name `xml:"SyncInformation"`
	Name    string   `xml:"Name"`
	Version int      `xml:"Version"`
	Lists   *Lists   `xml:"Lists,omitempty"`
}

// Lists contains all reading lists
type Lists struct {
	List []ReadingList `xml:"List"`
}

// ReadingList represents a single reading list
type ReadingList struct {
	Name        string   `xml:"Name,attr"`
	Description string   `xml:"Description,omitempty"`
	Books       *BookIDs `xml:"Books,omitempty"`
}

// BookIDs contains comic book GUIDs
type BookIDs struct {
	ID []string `xml:"Id"`
}

// writeSyncInformation writes reading lists to sync_information.xml
func (s *Syncer) writeSyncInformation() error {
	syncInfo := SyncInformation{
		Name:    "ComicRack",
		Version: 1,
	}

	// Get reading lists to include in sync_information.xml
	// This includes:
	// 1. Regular reading lists from the library
	// 2. Smart lists that were used for filtering (converted to regular lists with actual book IDs)
	readingLists := s.getReadingLists()
	if len(readingLists) > 0 {
		syncInfo.Lists = &Lists{
			List: readingLists,
		}
	}

	// Marshal to XML
	data, err := xml.MarshalIndent(syncInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync information: %w", err)
	}

	// Add XML declaration
	xmlData := []byte(xml.Header + string(data))
	return s.client.WriteFile("sync_information.xml", xmlData)
}

// getReadingLists extracts reading lists from the library
// Includes:
// 1. Regular reading lists from the library
// 2. Smart lists used for filtering (converted to regular lists with synced book IDs)
func (s *Syncer) getReadingLists() []ReadingList {
	var lists []ReadingList

	// IDs of every list assigned to this device as a filter (smart list,
	// ID list, or reading list) - these are handled separately below with
	// their own settings-filtered membership, so the "regular reading
	// lists" pass must skip them too, not just smart lists. Missing this
	// for a non-smart-list type (e.g. a ComicIdListItem like "To Read")
	// wrote it into sync_information.xml TWICE - once here with its raw,
	// unfiltered membership, once below with the correct filtered one -
	// two <List> entries sharing a Name that the device app appears to
	// resolve by hiding the list entirely, even though its books still
	// transferred fine as regular Add operations. Found live 2026-08-27.
	filterListIDs := make(map[string]bool)
	for _, fl := range s.filterLists {
		if fl != nil {
			filterListIDs[fl.ID] = true
		}
	}
	if s.filterList != nil {
		filterListIDs[s.filterList.ID] = true
	}

	// Add regular reading lists from the library
	allLists, err := s.backend.GetAllLists()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get reading lists from backend")
		// Continue without library lists - we may still have filter lists
	} else {
		for _, listItem := range allLists {
			// Skip smart lists, and any list already handled as a filter
			// list below (see filterListIDs above).
			if listItem.Type == "comicrack:ComicSmartListItem" || filterListIDs[listItem.ID] {
				continue
			}

			readingList := ReadingList{
				Name:        listItem.Name,
				Description: "", // ComicListItem doesn't have description in current implementation
			}

			// Add book IDs from the list
			if len(listItem.Items) > 0 {
				readingList.Books = &BookIDs{
					ID: make([]string, 0, len(listItem.Items)),
				}
				for _, item := range listItem.Items {
					readingList.Books.ID = append(readingList.Books.ID, item.ID)
				}
			}

			lists = append(lists, readingList)
		}
	}

	// Add smart lists that were used for filtering
	// Convert them to regular reading lists with the actual book IDs that were synced
	// Check both filterLists (new) and filterList (old, backward compatibility)
	var activeFilterLists []*library.ComicListItem
	if len(s.filterLists) > 0 {
		activeFilterLists = s.filterLists
	} else if s.filterList != nil {
		activeFilterLists = []*library.ComicListItem{s.filterList}
	}

	if len(activeFilterLists) > 0 {
		// For each filter list, create a reading list entry containing
		// THAT list's own matched-and-settings-filtered books - not the
		// union across every list assigned to the device. Writing the
		// union into every entry (the previous behavior) made every
		// synced list look identical on the device regardless of how
		// different their actual membership was - see comic-server-f4i,
		// found live after enabling per-list only-unread settings that
		// differed between two lists but the device still showed them as
		// the same content.
		for _, filterList := range activeFilterLists {
			booksForThisList, err := s.booksForFilterList(filterList)
			if err != nil {
				log.Error().Err(err).Str("list_name", filterList.Name).Msg("Failed to compute list's own books for sync_information.xml")
				booksForThisList = nil
			}

			readingList := ReadingList{
				Name:        filterList.Name,
				Description: "",
			}

			// Add book IDs from the filtered books
			if len(booksForThisList) > 0 {
				readingList.Books = &BookIDs{
					ID: make([]string, 0, len(booksForThisList)),
				}
				for _, book := range booksForThisList {
					readingList.Books.ID = append(readingList.Books.ID, book.ID)
				}
			}

			lists = append(lists, readingList)
			log.Debug().
				Str("list_name", filterList.Name).
				Int("book_count", len(booksForThisList)).
				Msg("Added smart list to sync_information.xml")
		}
	}

	return lists
}

// getTitleForOp extracts a displayable title from a sync operation
func getTitleForOp(op SyncOperation) string {
	if op.Book != nil {
		if op.Book.Title != "" {
			return op.Book.Title
		}
		// Use filename if title is empty
		if op.Book.FilePath != "" {
			baseFilename := filepath.Base(op.Book.FilePath)
			return strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))
		}
		return op.Book.ID
	}
	if op.Device != nil {
		if op.Device.Metadata != nil && op.Device.Metadata.Title != "" {
			return op.Device.Metadata.Title
		}
		// Use filename without .cbp extension
		return strings.TrimSuffix(op.Device.Filename, ".cbp")
	}
	return "(unknown)"
}

// readComicFile reads a comic book file from disk
// The file path is stored in the library metadata
func (s *Syncer) readComicFile(book *library.ComicBook) ([]byte, error) {
	if book.FilePath == "" {
		return nil, fmt.Errorf("book %s has no file path", book.ID)
	}

	resolvedPath := s.resolvePath(book.FilePath)
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read comic file %s: %w", resolvedPath, err)
	}

	return data, nil
}

// calculateRequiredSpace estimates the total space needed for sync operations
// Returns the total bytes required
func (s *Syncer) calculateRequiredSpace(operations []SyncOperation) (int64, error) {
	var totalBytes int64

	for _, op := range operations {
		switch op.Type {
		case OperationAdd, OperationUpdate:
			// Need space for both book file and sidecar
			if op.Book.FilePath != "" {
				resolvedPath := s.resolvePath(op.Book.FilePath)
				fileInfo, err := os.Stat(resolvedPath)
				if err != nil {
					// If we can't stat the file, estimate conservatively
					log.Warn().Err(err).Str("path", resolvedPath).Msg("Cannot stat file, using estimate")
					// Assume 50MB per book if we can't get size
					totalBytes += 50 * 1024 * 1024
					continue
				}
				totalBytes += fileInfo.Size()
			}
			// Add space for sidecar (typically small, ~10KB)
			totalBytes += 10 * 1024

		case OperationUpdateMetadataOnly:
			// Only need space for sidecar update
			totalBytes += 10 * 1024
			// OperationDelete frees space, so we don't count it
		}
	}

	return totalBytes, nil
}

// touchMarkerFile updates the marker file (comicrack.ini) timestamp to trigger device refresh
// This is critical for the Android app to detect changes and refresh the library display
func (s *Syncer) touchMarkerFile() error {
	const markerFile = "comicrack.ini"

	// Read the current marker file
	data, err := s.client.ReadFile(markerFile)
	if err != nil {
		return fmt.Errorf("failed to read marker file: %w", err)
	}

	// Write it back (same contents, updates timestamp)
	if err := s.client.WriteFile(markerFile, data); err != nil {
		return fmt.Errorf("failed to write marker file: %w", err)
	}

	return nil
}

// sortOperationsByType orders operations for safer sync execution.
// Order: ADD → UPDATE/UpdateMetadataOnly → DELETE
//
// This ensures that if sync fails partway through:
// - New content is added first (safest)
// - Updates are applied (old deleted, new added atomically)
// - Deletions happen last (only removes content user doesn't want)
//
// If sync fails during an UPDATE after deletion, the next sync will
// detect the book is missing and re-add it.
func sortOperationsByType(operations []SyncOperation) []SyncOperation {
	var adds []SyncOperation
	var updates []SyncOperation
	var deletes []SyncOperation

	for _, op := range operations {
		switch op.Type {
		case OperationAdd:
			adds = append(adds, op)
		case OperationUpdate, OperationUpdateMetadataOnly:
			updates = append(updates, op)
		case OperationDelete:
			deletes = append(deletes, op)
		}
	}

	// Combine in safe order: ADD, UPDATE, DELETE
	result := make([]SyncOperation, 0, len(operations))
	result = append(result, adds...)
	result = append(result, updates...)
	result = append(result, deletes...)

	return result
}

// updateLibraryReadingState updates the local library with user-editable metadata from device books
// This implements client-to-server sync (reverse sync) for:
// - Reading progress (CurrentPage, LastPageRead, OpenCount, OpenedTime)
// - User ratings (Rating)
// - User annotations (Notes, Review, Summary, Tags)
// - Page metadata (page types, bookmarks)
// - Other user tracking (Checked flag)
func (s *Syncer) updateLibraryReadingState(deviceBooks map[string]*DeviceBook) error {
	// Skip if backend can't persist changes (e.g., no library path configured)
	if !s.backend.CanPersist() {
		log.Debug().Msg("Backend cannot persist changes, skipping library update")
		return nil
	}

	updatedCount := 0
	changedBookIDs := []string{} // Track which books changed for cache marking
	var updatedBooks []*library.ComicBook

	for bookID, deviceBook := range deviceBooks {
		// Skip if device book has no metadata
		if deviceBook.Metadata == nil {
			continue
		}

		// Find corresponding library book
		libraryBook, err := s.backend.GetBook(bookID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("book_id", bookID).
				Msg("Error getting library book, skipping metadata update")
			continue
		}
		if libraryBook == nil {
			// Device has a book that's not in library (might have been deleted)
			log.Debug().
				Str("book_id", bookID).
				Str("device_title", deviceBook.Metadata.Title).
				Msg("Device book not found in library, skipping metadata update")
			continue
		}

		// Track what changed for logging
		hasChanges := false
		changedFields := []string{}

		// Update reading progress fields
		if deviceBook.Metadata.CurrentPage != libraryBook.CurrentPage {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Int("old_page", libraryBook.CurrentPage).
				Int("new_page", deviceBook.Metadata.CurrentPage).
				Msg("Updating CurrentPage from device")
			libraryBook.CurrentPage = deviceBook.Metadata.CurrentPage
			hasChanges = true
			changedFields = append(changedFields, "CurrentPage")
		}

		if deviceBook.Metadata.LastPageRead != libraryBook.LastPageRead {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Int("old_last_page", libraryBook.LastPageRead).
				Int("new_last_page", deviceBook.Metadata.LastPageRead).
				Msg("Updating LastPageRead from device")
			libraryBook.LastPageRead = deviceBook.Metadata.LastPageRead
			hasChanges = true
			changedFields = append(changedFields, "LastPageRead")
		}

		if deviceBook.Metadata.OpenCount != libraryBook.OpenCount {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Int("old_count", libraryBook.OpenCount).
				Int("new_count", deviceBook.Metadata.OpenCount).
				Msg("Updating OpenCount from device")
			libraryBook.OpenCount = deviceBook.Metadata.OpenCount
			hasChanges = true
			changedFields = append(changedFields, "OpenCount")
		}

		if !deviceBook.Metadata.OpenedTime.IsZero() &&
			!deviceBook.Metadata.OpenedTime.Equal(libraryBook.OpenedTime.Time) {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Time("old_time", libraryBook.OpenedTime.Time).
				Time("new_time", deviceBook.Metadata.OpenedTime.Time).
				Msg("Updating OpenedTime from device")
			libraryBook.OpenedTime = deviceBook.Metadata.OpenedTime
			hasChanges = true
			changedFields = append(changedFields, "OpenedTime")
		}

		// Update user rating
		if deviceBook.Metadata.Rating != libraryBook.Rating {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Float64("old_rating", libraryBook.Rating).
				Float64("new_rating", deviceBook.Metadata.Rating).
				Msg("Updating Rating from device")
			libraryBook.Rating = deviceBook.Metadata.Rating
			hasChanges = true
			changedFields = append(changedFields, "Rating")
		}

		// Update user annotations
		if deviceBook.Metadata.Notes != libraryBook.Notes {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Msg("Updating Notes from device")
			libraryBook.Notes = deviceBook.Metadata.Notes
			hasChanges = true
			changedFields = append(changedFields, "Notes")
		}

		if deviceBook.Metadata.Review != libraryBook.Review {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Msg("Updating Review from device")
			libraryBook.Review = deviceBook.Metadata.Review
			hasChanges = true
			changedFields = append(changedFields, "Review")
		}

		if deviceBook.Metadata.Summary != libraryBook.Summary {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Msg("Updating Summary from device")
			libraryBook.Summary = deviceBook.Metadata.Summary
			hasChanges = true
			changedFields = append(changedFields, "Summary")
		}

		if deviceBook.Metadata.Tags != libraryBook.Tags {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Str("old_tags", libraryBook.Tags).
				Str("new_tags", deviceBook.Metadata.Tags).
				Msg("Updating Tags from device")
			libraryBook.Tags = deviceBook.Metadata.Tags
			hasChanges = true
			changedFields = append(changedFields, "Tags")
		}

		// Update checked flag
		if deviceBook.Metadata.Checked != libraryBook.Checked {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Bool("old_checked", libraryBook.Checked).
				Bool("new_checked", deviceBook.Metadata.Checked).
				Msg("Updating Checked flag from device")
			libraryBook.Checked = deviceBook.Metadata.Checked
			hasChanges = true
			changedFields = append(changedFields, "Checked")
		}

		// Update page metadata (page types, bookmarks, etc.)
		if s.hasPagesMetadataChanged(libraryBook, deviceBook.Metadata) {
			log.Debug().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Int("old_page_count", len(libraryBook.Pages)).
				Int("new_page_count", len(deviceBook.Metadata.Pages)).
				Msg("Updating page metadata from device")
			libraryBook.Pages = deviceBook.Metadata.Pages
			hasChanges = true
			changedFields = append(changedFields, "Pages")
		}

		if hasChanges {
			updatedCount++
			changedBookIDs = append(changedBookIDs, bookID)
			updatedBooks = append(updatedBooks, libraryBook)
			log.Info().
				Str("book_id", bookID).
				Str("title", libraryBook.Title).
				Strs("changed_fields", changedFields).
				Msg("Updated book metadata from device")
		}
	}

	if updatedCount > 0 {
		log.Info().
			Int("updated_count", updatedCount).
			Msg("Updated library metadata from device")

		// Update books in backend
		if err := s.backend.UpdateBooks(updatedBooks); err != nil {
			return fmt.Errorf("failed to update books: %w", err)
		}

		// Mark books as dirty for cache flushing (if backend supports it)
		s.backend.MarkManyDirty(changedBookIDs)

		// Flush changes to persistent storage
		if err := s.backend.Flush(); err != nil {
			return fmt.Errorf("failed to flush changes: %w", err)
		}

		log.Info().Msg("Library changes saved successfully")
	} else {
		log.Debug().Msg("No metadata changes detected")
	}

	return nil
}

// hasPagesMetadataChanged compares page-level metadata between library and device books
func (s *Syncer) hasPagesMetadataChanged(library, device *library.ComicBook) bool {
	// If counts differ, metadata changed
	if len(library.Pages) != len(device.Pages) {
		return true
	}

	// Compare each page's metadata
	for i := range library.Pages {
		libPage := library.Pages[i]
		devPage := device.Pages[i]

		// Check if any page fields differ
		if libPage.Image != devPage.Image ||
			libPage.Type != devPage.Type ||
			libPage.Bookmark != devPage.Bookmark ||
			libPage.ImageSize != devPage.ImageSize ||
			libPage.ImageWidth != devPage.ImageWidth ||
			libPage.ImageHeight != devPage.ImageHeight ||
			libPage.Key != devPage.Key {
			return true
		}
	}

	return false
}
