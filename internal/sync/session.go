package sync

import (
	"encoding/xml"
	"fmt"
	"log"

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

	// TODO: Calculate required space and validate it's sufficient

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
	// 1. Check if file exists (shouldn't, but be safe)
	filename := fmt.Sprintf("%s.cbp", book.ID)
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
	// TODO: Generate .cbp file from library book (Phase 2)
	// For now, just write sidecar
	// bookData, err := s.generateCBP(book)
	// if err != nil {
	// 	return fmt.Errorf("failed to generate CBP: %w", err)
	// }
	// if err := s.client.WriteFile(filename, bookData); err != nil {
	// 	return fmt.Errorf("failed to write book file: %w", err)
	// }

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

// writeSyncInformation writes reading lists to sync_information.xml
func (s *Syncer) writeSyncInformation() error {
	// TODO: Generate proper sync_information.xml with reading lists
	// For now, write empty/placeholder file
	syncInfo := `<?xml version="1.0" encoding="utf-8"?>
<SyncInformation>
</SyncInformation>
`
	return s.client.WriteFile("sync_information.xml", []byte(syncInfo))
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
