package library

import (
	"fmt"
	"sync"
	"time"
)

// XMLBackend implements Backend using the in-memory ComicLibrary with XML persistence.
type XMLBackend struct {
	mu          sync.RWMutex
	library     *ComicLibrary
	cache       *LibraryCache
	libraryPath string
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

	if b.cache != nil && len(updatedIDs) > 0 {
		b.cache.MarkManyDirty(updatedIDs)
	}

	return nil
}

// MarkDirty marks a book as modified for cache tracking.
func (b *XMLBackend) MarkDirty(bookID string) {
	if b.cache != nil {
		b.cache.MarkDirty(bookID)
	}
}

// MarkManyDirty marks multiple books as modified.
func (b *XMLBackend) MarkManyDirty(bookIDs []string) {
	if b.cache != nil {
		b.cache.MarkManyDirty(bookIDs)
	}
}

// Flush writes any pending changes to disk.
func (b *XMLBackend) Flush() error {
	if b.cache != nil {
		_, err := b.cache.Flush()
		return err
	}

	// No cache - save directly
	b.mu.RLock()
	defer b.mu.RUnlock()
	return SaveLibrary(b.libraryPath, b.library)
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
