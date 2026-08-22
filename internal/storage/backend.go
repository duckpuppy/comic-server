package storage

import (
	"fmt"
	"sync"

	"github.com/duckpuppy/comic-server/internal/library"
)

// SQLiteBackend implements library.Backend using SQLite for persistence.
type SQLiteBackend struct {
	mu     sync.RWMutex
	db     *DB
	dbPath string
	// xmlPath is the ComicRack XML this database was imported from, if
	// known. Empty if the database was opened standalone (e.g. via the
	// db-info CLI command) with no known source to reimport from - Reload
	// returns an error in that case rather than silently no-op'ing.
	xmlPath  string
	metadata struct {
		id   string
		name string
	}
	// cvData holds optional ComicVine enrichment data keyed by book ID, set
	// via SetCVData and attached to the temporary library MatchBooks/
	// GetBooksForList build for evaluation - see comic-server-22c.
	cvData map[string]*library.CVCompleteness
}

// NewSQLiteBackend creates a new SQLite-based backend. xmlPath is the
// library XML this database should be kept in sync with via Reload; pass
// "" if there's no source to reimport from (Reload will then error).
func NewSQLiteBackend(dbPath string, xmlPath string) (*SQLiteBackend, error) {
	db, err := Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	backend := &SQLiteBackend{
		db:      db,
		dbPath:  dbPath,
		xmlPath: xmlPath,
	}

	// Load library metadata
	if err := backend.loadMetadata(); err != nil {
		db.Close()
		return nil, fmt.Errorf("load metadata: %w", err)
	}

	return backend, nil
}

// Reload re-imports xmlPath (see NewSQLiteBackend) into the database in
// place, picking up any external changes (books/lists added, edited, or
// removed in ComicRack) without a process restart. The import is
// idempotent, so this is safe to call repeatedly - only rows that actually
// changed since the last import are touched.
//
// Safe to call while the server is running: reads only block for the
// duration of the import transaction, not the whole reload.
func (b *SQLiteBackend) Reload() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.xmlPath == "" {
		return fmt.Errorf("reload: no XML source path configured for this database")
	}

	lib, err := library.LoadLibrary(b.xmlPath)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}

	if _, err := b.db.Import(lib, ImportOptions{}); err != nil {
		return fmt.Errorf("reload: import: %w", err)
	}

	return b.loadMetadata()
}

func (b *SQLiteBackend) loadMetadata() error {
	// Try to load library_id
	var id, name string
	err := b.db.QueryRow("SELECT value FROM library_metadata WHERE key = 'library_id'").Scan(&id)
	if err == nil {
		b.metadata.id = id
	}

	err = b.db.QueryRow("SELECT value FROM library_metadata WHERE key = 'library_name'").Scan(&name)
	if err == nil {
		b.metadata.name = name
	}

	return nil
}

// GetBook retrieves a single book by ID.
func (b *SQLiteBackend) GetBook(id string) (*library.ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.db.GetBook(id)
}

// GetAllBooks returns all books in the library.
func (b *SQLiteBackend) GetAllBooks() ([]library.ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.db.GetAllBooks()
}

// FindListByID finds a list by its ID.
func (b *SQLiteBackend) FindListByID(id string) (*library.ComicListItem, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.db.GetList(id)
}

// FindList finds a list by name.
func (b *SQLiteBackend) FindList(name string) (*library.ComicListItem, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Get all lists and search by name
	lists, err := b.db.GetAllLists()
	if err != nil {
		return nil, err
	}

	return findListByNameRecursive(lists, name), nil
}

func findListByNameRecursive(lists []library.ComicListItem, name string) *library.ComicListItem {
	for i := range lists {
		if lists[i].Name == name {
			return &lists[i]
		}
		if found := findListByNameRecursive(lists[i].ChildItems, name); found != nil {
			return found
		}
	}
	return nil
}

// GetAllLists returns all top-level lists.
func (b *SQLiteBackend) GetAllLists() ([]library.ComicListItem, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.db.GetAllLists()
}

// CreateList inserts a new list into the database.
func (b *SQLiteBackend) CreateList(list *library.ComicListItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.db.InsertList(list)
}

// UpdateList updates an existing list in the database.
func (b *SQLiteBackend) UpdateList(list *library.ComicListItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.db.UpdateListRecord(list)
}

// DeleteList removes a list and its children from the database.
func (b *SQLiteBackend) DeleteList(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.db.DeleteList(id)
}

// MoveList updates the parent_id of a list or folder. parentID="" moves to root.
func (b *SQLiteBackend) MoveList(id, parentID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.db.MoveList(id, parentID)
}

// MatchBooks evaluates a smart list and returns matching books.
func (b *SQLiteBackend) MatchBooks(list *library.ComicListItem) ([]*library.ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	tempLib, err := b.tempLibraryLocked()
	if err != nil {
		return nil, err
	}
	return tempLib.MatchBooks(list)
}

// GetBooksForList returns books for any list type (SmartList, IdList, ReadingList).
func (b *SQLiteBackend) GetBooksForList(list *library.ComicListItem) ([]*library.ComicBook, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	tempLib, err := b.tempLibraryLocked()
	if err != nil {
		return nil, err
	}
	return tempLib.GetBooksForList(list)
}

// tempLibraryLocked builds an in-memory library.ComicLibrary snapshot for
// matcher evaluation. Includes ComicLists (not just Books) so BaseListId
// scoping resolves via FindListByID the same way it does on XMLBackend -
// see comic-server-hha. Caller must hold at least b.mu.RLock().
//
// TODO: Could optimize with SQL queries for simple matchers instead of
// loading everything into memory (comic-server-770).
func (b *SQLiteBackend) tempLibraryLocked() (*library.ComicLibrary, error) {
	books, err := b.db.GetAllBooks()
	if err != nil {
		return nil, err
	}
	lists, err := b.db.GetAllLists()
	if err != nil {
		return nil, err
	}

	tempLib := &library.ComicLibrary{Books: books, ComicLists: lists}
	tempLib.SetCVData(b.cvData)
	return tempLib, nil
}

// SetCVData sets ComicVine enrichment data for use by CV smart list matchers
// (CVSeriesComplete, CVMissingCount, CVPercentOwned). Mirrors XMLBackend.SetCVData.
func (b *SQLiteBackend) SetCVData(data map[string]*library.CVCompleteness) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cvData = data
}

// UpdateBook updates a single book in the database.
func (b *SQLiteBackend) UpdateBook(book *library.ComicBook) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.db.UpdateBookFields(book)
}

// UpdateBooks updates multiple books in the database.
func (b *SQLiteBackend) UpdateBooks(books []*library.ComicBook) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Update each book (SQLite handles this efficiently)
	for _, book := range books {
		if err := b.db.UpdateBookFields(book); err != nil {
			return err
		}
	}

	return nil
}

// MarkDirty is a no-op for SQLite (changes are written immediately).
func (b *SQLiteBackend) MarkDirty(bookID string) {
	// SQLite writes are immediate, no dirty tracking needed
}

// MarkManyDirty is a no-op for SQLite.
func (b *SQLiteBackend) MarkManyDirty(bookIDs []string) {
	// SQLite writes are immediate, no dirty tracking needed
}

// Flush is a no-op for SQLite (changes are written immediately).
func (b *SQLiteBackend) Flush() error {
	// SQLite writes are immediate
	return nil
}

// Close closes the database connection.
func (b *SQLiteBackend) Close() error {
	return b.db.Close()
}

// LibraryID returns the library's unique identifier.
func (b *SQLiteBackend) LibraryID() string {
	return b.metadata.id
}

// LibraryName returns the library's display name.
func (b *SQLiteBackend) LibraryName() string {
	return b.metadata.name
}

// BookCount returns the total number of books.
func (b *SQLiteBackend) BookCount() int {
	count, _ := b.db.GetBookCount()
	return count
}

// CanPersist returns true because SQLite always persists changes.
func (b *SQLiteBackend) CanPersist() bool {
	return true
}

// DB returns the underlying database connection (for advanced operations).
func (b *SQLiteBackend) DB() *DB {
	return b.db
}
