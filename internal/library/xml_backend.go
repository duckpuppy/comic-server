package library

import (
	"fmt"
	"sync"
	"time"
)

// XMLBackend implements Backend using the in-memory ComicLibrary with XML persistence.
type XMLBackend struct {
	mu            sync.RWMutex
	library       *ComicLibrary
	cache         *LibraryCache
	libraryPath   string
	dirty         bool      // tracks unsaved book changes when no LibraryCache is configured
	lastWriteTime time.Time // last time this backend wrote libraryPath, when cache is nil (see LastWriteTime)
}

// NewXMLBackend creates a new XML-based backend from a library file.
func NewXMLBackend(libraryPath string, flushInterval time.Duration) (*XMLBackend, error) {
	lib, err := LoadLibrary(libraryPath)
	if err != nil {
		return nil, fmt.Errorf("load library: %w", err)
	}

	backend := &XMLBackend{
		library:     lib,
		libraryPath: libraryPath,
	}

	// Create cache if flush interval is set
	if flushInterval > 0 {
		backend.cache = NewLibraryCache(lib, libraryPath, flushInterval)
	}

	return backend, nil
}

// NewXMLBackendFromLibrary creates a backend from an existing loaded library.
func NewXMLBackendFromLibrary(lib *ComicLibrary, libraryPath string, cache *LibraryCache) *XMLBackend {
	return &XMLBackend{
		library:     lib,
		cache:       cache,
		libraryPath: libraryPath,
	}
}

// GetBook retrieves a single book by ID.
func (b *XMLBackend) GetBook(id string) (*ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	book := b.library.GetBook(id)
	if book == nil {
		return nil, nil
	}
	return book, nil
}

// GetAllBooks returns all books in the library.
func (b *XMLBackend) GetAllBooks() ([]ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Return a copy to prevent external modification
	books := make([]ComicBook, len(b.library.Books))
	copy(books, b.library.Books)
	return books, nil
}

// FindListByID finds a list by its ID (recursive search).
func (b *XMLBackend) FindListByID(id string) (*ComicListItem, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	list := b.library.FindListByID(id)
	return list, nil
}

// FindList finds a list by name.
func (b *XMLBackend) FindList(name string) (*ComicListItem, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	list := b.library.FindList(name)
	return list, nil
}

// GetAllLists returns all top-level lists.
func (b *XMLBackend) GetAllLists() ([]ComicListItem, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Return a copy
	lists := make([]ComicListItem, len(b.library.ComicLists))
	copy(lists, b.library.ComicLists)
	return lists, nil
}

// CreateList adds a new list to the library and persists it.
func (b *XMLBackend) CreateList(list *ComicListItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure no duplicate IDs
	if b.library.FindListByID(list.ID) != nil {
		return fmt.Errorf("list with ID %s already exists", list.ID)
	}

	b.library.ComicLists = append(b.library.ComicLists, *list)
	return b.flushLocked()
}

// UpdateList replaces the list with the matching ID and persists.
func (b *XMLBackend) UpdateList(list *ComicListItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if updateListRecursive(b.library.ComicLists, list) {
		return b.flushLocked()
	}
	return fmt.Errorf("list not found: %s", list.ID)
}

// DeleteList removes a list by ID and persists.
func (b *XMLBackend) DeleteList(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	original := len(b.library.ComicLists)
	b.library.ComicLists = deleteListRecursive(b.library.ComicLists, id)
	if len(b.library.ComicLists) == original {
		return fmt.Errorf("list not found: %s", id)
	}
	return b.flushLocked()
}

// MoveList moves a list or folder to a new parent. parentID="" moves to root.
func (b *XMLBackend) MoveList(id, parentID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Extract the item from its current location
	var extracted *ComicListItem
	b.library.ComicLists, extracted = extractListRecursive(b.library.ComicLists, id)
	if extracted == nil {
		return fmt.Errorf("list not found: %s", id)
	}

	if parentID == "" {
		// Move to root
		b.library.ComicLists = append(b.library.ComicLists, *extracted)
	} else {
		// Insert under the new parent
		if !insertIntoParentRecursive(b.library.ComicLists, parentID, *extracted) {
			// Parent not found — put it back and return error
			b.library.ComicLists = append(b.library.ComicLists, *extracted)
			return fmt.Errorf("parent not found: %s", parentID)
		}
	}

	return b.flushLocked()
}

// extractListRecursive removes the item with the given id from lists and returns it.
func extractListRecursive(lists []ComicListItem, id string) ([]ComicListItem, *ComicListItem) {
	result := make([]ComicListItem, 0, len(lists))
	var found *ComicListItem
	for i := range lists {
		if lists[i].ID == id {
			cp := lists[i]
			found = &cp
			continue
		}
		item := lists[i]
		item.ChildItems, _ = extractListRecursive(item.ChildItems, id)
		result = append(result, item)
	}
	return result, found
}

// insertIntoParentRecursive inserts child under the folder with parentID.
func insertIntoParentRecursive(lists []ComicListItem, parentID string, child ComicListItem) bool {
	for i := range lists {
		if lists[i].ID == parentID {
			lists[i].ChildItems = append(lists[i].ChildItems, child)
			return true
		}
		if insertIntoParentRecursive(lists[i].ChildItems, parentID, child) {
			return true
		}
	}
	return false
}

// flushLocked saves the library to disk. Caller must hold mu.Lock().
func (b *XMLBackend) flushLocked() error {
	if b.libraryPath == "" {
		return nil
	}
	return SaveLibrary(b.libraryPath, b.library)
}

func updateListRecursive(lists []ComicListItem, update *ComicListItem) bool {
	for i := range lists {
		if lists[i].ID == update.ID {
			lists[i] = *update
			return true
		}
		if updateListRecursive(lists[i].ChildItems, update) {
			return true
		}
	}
	return false
}

func deleteListRecursive(lists []ComicListItem, id string) []ComicListItem {
	result := lists[:0]
	for _, item := range lists {
		if item.ID == id {
			continue
		}
		item.ChildItems = deleteListRecursive(item.ChildItems, id)
		result = append(result, item)
	}
	return result
}

// SetCVData sets ComicVine enrichment data for use by CV smart list matchers.
func (b *XMLBackend) SetCVData(data map[string]*CVCompleteness) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.library.SetCVData(data)
}

// MatchBooks evaluates a smart list and returns matching books.
func (b *XMLBackend) MatchBooks(list *ComicListItem) ([]*ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.library.MatchBooks(list)
}

// GetBooksForList returns books for any list type (SmartList, IdList, ReadingList).
func (b *XMLBackend) GetBooksForList(list *ComicListItem) ([]*ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.library.GetBooksForList(list)
}

// UpdateBook updates a single book in the library.
func (b *XMLBackend) UpdateBook(book *ComicBook) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find and update the book in the library
	for i := range b.library.Books {
		if b.library.Books[i].ID == book.ID {
			b.library.Books[i] = *book
			if b.cache != nil {
				b.cache.MarkDirty(book.ID)
			} else {
				b.dirty = true
			}
			return nil
		}
	}

	return fmt.Errorf("book not found: %s", book.ID)
}

// UpdateBooks updates multiple books in the library.
func (b *XMLBackend) UpdateBooks(books []*ComicBook) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Build a map for efficient lookup
	updates := make(map[string]*ComicBook)
	for _, book := range books {
		updates[book.ID] = book
	}

	// Update books in place
	var updatedIDs []string
	for i := range b.library.Books {
		if update, ok := updates[b.library.Books[i].ID]; ok {
			b.library.Books[i] = *update
			updatedIDs = append(updatedIDs, update.ID)
		}
	}

	if b.cache != nil {
		if len(updatedIDs) > 0 {
			b.cache.MarkManyDirty(updatedIDs)
		}
	} else if len(updatedIDs) > 0 {
		b.dirty = true
	}

	return nil
}

// MarkDirty marks a book as modified for cache tracking.
func (b *XMLBackend) MarkDirty(bookID string) {
	if b.cache != nil {
		b.cache.MarkDirty(bookID)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dirty = true
}

// MarkManyDirty marks multiple books as modified.
func (b *XMLBackend) MarkManyDirty(bookIDs []string) {
	if b.cache != nil {
		b.cache.MarkManyDirty(bookIDs)
		return
	}
	if len(bookIDs) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dirty = true
}

// Flush writes any pending changes to disk. When no LibraryCache is
// configured, this is a no-op unless a book was actually changed since the
// last flush (via UpdateBook/UpdateBooks/MarkDirty/MarkManyDirty).
func (b *XMLBackend) Flush() error {
	if b.cache != nil {
		_, err := b.cache.Flush()
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.dirty || b.libraryPath == "" {
		return nil
	}
	if err := SaveLibrary(b.libraryPath, b.library); err != nil {
		return err
	}
	b.dirty = false
	b.lastWriteTime = time.Now()
	return nil
}

// LastWriteTime returns the last time this backend wrote libraryPath to
// disk (via Flush, or the LibraryCache's own periodic auto-flush). Used by
// Watcher to distinguish comic-server's own writes from external changes -
// a file-change event that follows shortly after LastWriteTime is assumed
// to be an echo of our own write, not new data to reload.
func (b *XMLBackend) LastWriteTime() time.Time {
	if b.cache != nil {
		return b.cache.LastSaveTime()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastWriteTime
}

// Reload re-reads the library file from disk, replacing the in-memory
// library. Any pending dirty changes are flushed to disk first, so a
// reload never silently discards comic-server's own unsaved writes -
// though note this means comic-server's own pending edits always win over
// a concurrent external edit to the same book, since they're written
// before the reload reads the file back.
//
// Safe to call while the server is running: GetBook/MatchBooks/etc. only
// block for the brief moment the library pointer is swapped.
func (b *XMLBackend) Reload() error {
	if err := b.Flush(); err != nil {
		return fmt.Errorf("reload: flush pending changes: %w", err)
	}

	b.mu.RLock()
	path := b.libraryPath
	b.mu.RUnlock()
	if path == "" {
		return fmt.Errorf("reload: no library path configured")
	}

	lib, err := LoadLibrary(path)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}

	b.mu.Lock()
	b.library = lib
	b.dirty = false
	if b.cache != nil {
		b.cache.SetLibrary(lib)
	}
	b.mu.Unlock()

	return nil
}

// Close stops the backend and flushes any pending changes.
func (b *XMLBackend) Close() error {
	if b.cache != nil {
		return b.cache.Stop()
	}
	return b.Flush()
}

// LibraryID returns the library's unique identifier.
func (b *XMLBackend) LibraryID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.library.ID
}

// LibraryName returns the library's display name.
func (b *XMLBackend) LibraryName() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.library.Name
}

// BookCount returns the total number of books.
func (b *XMLBackend) BookCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.library.Books)
}

// CanPersist returns true if changes can be saved to persistent storage.
func (b *XMLBackend) CanPersist() bool {
	return b.libraryPath != ""
}

// Library returns the underlying ComicLibrary (for backwards compatibility).
// Deprecated: Use Backend interface methods instead.
func (b *XMLBackend) Library() *ComicLibrary {
	return b.library
}

// Cache returns the underlying LibraryCache (for backwards compatibility).
// Deprecated: Use Backend interface methods instead.
func (b *XMLBackend) Cache() *LibraryCache {
	return b.cache
}
