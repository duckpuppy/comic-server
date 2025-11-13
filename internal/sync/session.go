package sync

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// PerformSync executes a full synchronization session with a device
// Follows the sync flow from WIRELESS_SYNC_PROTOCOL.md:864-912
func (s *Syncer) PerformSync() (*SyncResult, error) {
	result := &SyncResult{
		Errors: make([]error, 0),
	}

	// Step 1: CommandStart - Begin sync session
	log.Println("Starting synchronization session...")
	if err := s.client.SendStart(); err != nil {
		return result, fmt.Errorf("failed to start sync: %w", err)
	}

	// Step 2: CommandInfo - Validate device
	log.Println("Validating device...")
	deviceInfo, err := s.client.GetDeviceInfo()
	if err != nil {
		return result, fmt.Errorf("failed to get device info: %w", err)
	}
	log.Printf("Device validated: Licensed=%v, Version=%d\n", deviceInfo.Licensed, deviceInfo.VersionCode)

	// Step 3: CommandFreeSpace - Check available storage
	log.Println("Checking device storage...")
	freeSpace, err := s.client.GetFreeSpace()
	if err != nil {
		return result, fmt.Errorf("failed to get free space: %w", err)
	}
	log.Printf("Device free space: %d bytes (%.2f MB)\n", freeSpace, float64(freeSpace)/(1024*1024))

	// Step 4: Get current device state
	log.Println("Retrieving device book list...")
	deviceBooks, err := s.GetDeviceBooks()
	if err != nil {
		return result, fmt.Errorf("failed to get device books: %w", err)
	}
	log.Printf("Found %d books on device\n", len(deviceBooks))

	// Step 5: Compute sync plan
	log.Println("Computing sync plan...")
	operations, err := s.ComputeSyncPlan(deviceBooks)
	if err != nil {
		return result, fmt.Errorf("failed to compute sync plan: %w", err)
	}
	log.Printf("Sync plan: %d operations\n", len(operations))

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
	log.Printf("  Add: %d, Update: %d, Delete: %d, Metadata-only: %d\n",
		addCount, updateCount, deleteCount, metadataOnlyCount)

	// Validate storage space
	requiredSpace, err := s.calculateRequiredSpace(operations)
	if err != nil {
		return result, fmt.Errorf("failed to calculate required space: %w", err)
	}
	log.Printf("Required space: %d bytes (%.2f MB)\n", requiredSpace, float64(requiredSpace)/(1024*1024))

	// Add 10% buffer for overhead
	requiredSpaceWithBuffer := int64(float64(requiredSpace) * 1.1)
	if requiredSpaceWithBuffer > freeSpace {
		return result, fmt.Errorf("insufficient storage: need %.2f MB, have %.2f MB",
			float64(requiredSpaceWithBuffer)/(1024*1024),
			float64(freeSpace)/(1024*1024))
	}

	// Step 6: Execute sync operations
	totalOps := len(operations)
	for i, op := range operations {
		// Check for abort periodically
		if i%10 == 0 {
			aborted, err := s.client.CheckAbort()
			if err != nil {
				log.Printf("Warning: failed to check abort status: %v\n", err)
			} else if aborted {
				return result, fmt.Errorf("sync aborted by user")
			}
		}

		// Execute operation
		if err := s.executeOperation(op); err != nil {
			errMsg := fmt.Errorf("operation %d/%d failed (%s for %s): %w",
				i+1, totalOps, op.Type, op.Book.Title, err)
			result.Errors = append(result.Errors, errMsg)
			log.Printf("Error: %v\n", errMsg)
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
			log.Printf("Warning: failed to send progress update: %v\n", err)
		}

		log.Printf("  [%d/%d] %s: %s\n", i+1, totalOps, op.Type, getTitleForOp(op))
	}

	// Step 7: Write sync_information.xml (reading lists)
	log.Println("Writing reading lists...")
	if err := s.writeSyncInformation(); err != nil {
		// Don't fail sync if reading list write fails
		log.Printf("Warning: failed to write reading lists: %v\n", err)
	}

	// Step 8: CommandCompleted - Signal sync completion
	log.Println("Completing synchronization...")
	if err := s.client.SendCompleted(); err != nil {
		return result, fmt.Errorf("failed to send completion: %w", err)
	}

	// Step 9: Final progress update
	if err := s.client.SendProgressUpdate(100); err != nil {
		log.Printf("Warning: failed to send final progress: %v\n", err)
	}

	log.Printf("Sync complete: +%d ~%d -%d books, %d errors\n",
		result.BooksAdded, result.BooksUpdated, result.BooksDeleted, len(result.Errors))

	return result, nil
}

// executeOperation performs a single sync operation
func (s *Syncer) executeOperation(op SyncOperation) error {
	switch op.Type {
	case OperationAdd:
		return s.addBook(op.Book)
	case OperationUpdate:
		return s.updateBook(op.Book, op.Device)
	case OperationUpdateMetadataOnly:
		return s.updateMetadataOnly(op.Book, op.Device)
	case OperationDelete:
		return s.deleteBook(op.Device)
	default:
		return fmt.Errorf("unknown operation type: %v", op.Type)
	}
}

// addBook adds a new book to the device
func (s *Syncer) addBook(book *library.ComicBook) error {
	// 1. Determine target filename on device
	// Use actual filename from library, not GUID
	baseFilename := filepath.Base(book.FilePath)
	// Change extension to .cbp for device storage
	filename := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename)) + ".cbp"

	// 2. Check if file exists (shouldn't, but be safe)
	exists, err := s.client.FileExists(filename)
	if err != nil {
		return fmt.Errorf("failed to check if file exists: %w", err)
	}
	if exists {
		// File exists but wasn't in our book list - delete it first
		if err := s.client.DeleteFile(filename); err != nil {
			return fmt.Errorf("failed to delete existing file: %w", err)
		}
	}

	// 2. Write comic book file (.cbp)
	bookData, err := s.readComicFile(book)
	if err != nil {
		return fmt.Errorf("failed to read comic file: %w", err)
	}
	if err := s.client.WriteFile(filename, bookData); err != nil {
		return fmt.Errorf("failed to write book file: %w", err)
	}

	// 3. Write sidecar metadata (.cbp.xml)
	sidecarData, err := s.generateSidecar(book)
	if err != nil {
		return fmt.Errorf("failed to generate sidecar: %w", err)
	}
	sidecarFilename := filename + ".xml"
	if err := s.client.WriteFile(sidecarFilename, sidecarData); err != nil {
		return fmt.Errorf("failed to write sidecar: %w", err)
	}

	return nil
}

// updateBook updates an existing book on the device (full re-transfer)
func (s *Syncer) updateBook(book *library.ComicBook, device *DeviceBook) error {
	// Delete old files
	if err := s.deleteBook(device); err != nil {
		return fmt.Errorf("failed to delete old book: %w", err)
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
func (s *Syncer) deleteBook(device *DeviceBook) error {
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
	Name        string  `xml:"Name,attr"`
	Description string  `xml:"Description,omitempty"`
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

	// Get all reading lists from the library
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
// Only includes non-smart lists (regular reading lists)
func (s *Syncer) getReadingLists() []ReadingList {
	var lists []ReadingList

	for _, listItem := range s.library.ComicLists {
		// Skip smart lists - only sync regular reading lists
		if listItem.Type == "comicrack:ComicSmartListItem" {
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

	return lists
}

// getTitleForOp extracts a displayable title from a sync operation
func getTitleForOp(op SyncOperation) string {
	if op.Book != nil {
		if op.Book.Title != "" {
			return op.Book.Title
		}
		return op.Book.ID
	}
	if op.Device != nil {
		if op.Device.Metadata != nil && op.Device.Metadata.Title != "" {
			return op.Device.Metadata.Title
		}
		return op.Device.Filename
	}
	return "(unknown)"
}

// readComicFile reads a comic book file from disk
// The file path is stored in the library metadata
func (s *Syncer) readComicFile(book *library.ComicBook) ([]byte, error) {
	if book.FilePath == "" {
		return nil, fmt.Errorf("book %s has no file path", book.ID)
	}

	data, err := os.ReadFile(book.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read comic file %s: %w", book.FilePath, err)
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
				fileInfo, err := os.Stat(op.Book.FilePath)
				if err != nil {
					// If we can't stat the file, estimate conservatively
					log.Printf("Warning: cannot stat %s: %v", op.Book.FilePath, err)
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
