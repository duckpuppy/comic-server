package storage

import (
	"fmt"
	"strings"
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
	// libCache is a shared snapshot of every book and list, reused across
	// repeated calls to cachedLibrary() until invalidated by any write
	// (see invalidateCacheLocked) - comic-server-ea5. Without this,
	// every list evaluation that can't use the SQL fast path (matcher_sql.go)
	// rebuilt this from scratch - a full GetAllBooks() SQL fetch plus
	// batched tag/custom-value joins - which at real-library scale (67K
	// books, hundreds of untranslatable-matcher lists) turned a cold
	// list-tree warm-up (internal/api/lists.go evaluates every list
	// serially within one request) into a multi-hour stall: each list
	// paying the ~1-1.5s rebuild cost independently instead of once.
	libCache *library.ComicLibrary

	// warmMu guards generation/warmed, separately from mu, so a readiness
	// check (NotReadyLists) is a cheap in-memory read that never blocks
	// behind a slow reload or snapshot rebuild - see WarmUp/IsWarm
	// (comic-server-jrn).
	warmMu     sync.RWMutex
	generation int64
	warmed     bool
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

	b.libCache = nil
	b.bumpGeneration()
	return b.loadMetadata()
}

// bumpGeneration marks the shared library snapshot (libCache) as stale
// after a reload, so NotReadyLists starts reporting affected lists as not
// ready again until a subsequent WarmUp (or ordinary evaluation) rebuilds
// it. Called with b.mu already held (Reload).
func (b *SQLiteBackend) bumpGeneration() {
	b.warmMu.Lock()
	b.generation++
	b.warmed = false
	b.warmMu.Unlock()
}

// WarmUp rebuilds the shared full-library snapshot (internal cachedLibrary)
// if it isn't already fresh for the current reload generation, then marks
// the backend warm. Intended to run once in a background goroutine right
// after startup and after each Reload, so that a device sync attempt finds
// the snapshot already warm instead of paying its rebuild cost inline -
// see NotReadyLists (comic-server-jrn).
func (b *SQLiteBackend) WarmUp() error {
	b.warmMu.RLock()
	gen := b.generation
	b.warmMu.RUnlock()

	if _, err := b.cachedLibrary(); err != nil {
		return err
	}

	b.warmMu.Lock()
	// Only mark warm if no other reload happened while we were building -
	// otherwise this stale build's completion would incorrectly clear the
	// not-ready state for the NEW generation's (not yet rebuilt) snapshot.
	if b.generation == gen {
		b.warmed = true
	}
	b.warmMu.Unlock()
	return nil
}

// IsWarm reports whether the shared full-library snapshot is fresh for the
// current reload generation. A cheap in-memory read - never itself
// triggers or waits on a rebuild.
func (b *SQLiteBackend) IsWarm() bool {
	b.warmMu.RLock()
	defer b.warmMu.RUnlock()
	return b.warmed
}

// needsWarmSnapshot reports whether evaluating list would fall back to the
// shared full-library snapshot (cachedLibrary) rather than a scoped SQL
// query - mirrors evaluationLibrary's own branch, without actually running
// the query.
func (b *SQLiteBackend) needsWarmSnapshot(list *library.ComicListItem) bool {
	if list != nil && list.BaseListId == "" && strings.Contains(list.Type, "SmartList") {
		if _, ok := translateMatchers(list.MatcherMode, list.Matchers); ok {
			return false
		}
	}
	return true
}

// NotReadyLists checks the given (already-resolved) lists against the
// current warm-up state and returns the IDs of any that would trigger a
// slow, uncached evaluation right now - lists that need the shared
// snapshot while it's stale after a reload. An empty result means it's
// safe to evaluate all of them immediately (comic-server-jrn: used to
// hard-block a device sync until its assigned lists are ready, rather than
// risk the sync stalling on a cold evaluation).
func (b *SQLiteBackend) NotReadyLists(lists []*library.ComicListItem) []string {
	if b.IsWarm() {
		return nil
	}
	var notReady []string
	for _, l := range lists {
		if l != nil && b.needsWarmSnapshot(l) {
			notReady = append(notReady, l.ID)
		}
	}
	return notReady
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
	b.libCache = nil
	return b.db.InsertList(list)
}

// UpdateList updates an existing list in the database.
func (b *SQLiteBackend) UpdateList(list *library.ComicListItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.libCache = nil
	return b.db.UpdateListRecord(list)
}

// DeleteList removes a list and its children from the database.
func (b *SQLiteBackend) DeleteList(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.libCache = nil
	return b.db.DeleteList(id)
}

// MoveList updates the parent_id of a list or folder. parentID="" moves to root.
func (b *SQLiteBackend) MoveList(id, parentID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.libCache = nil
	return b.db.MoveList(id, parentID)
}

// MatchBooks evaluates a smart list and returns matching books.
func (b *SQLiteBackend) MatchBooks(list *library.ComicListItem) ([]*library.ComicBook, error) {
	tempLib, err := b.evaluationLibrary(list)
	if err != nil {
		return nil, err
	}
	return tempLib.MatchBooks(list)
}

// GetBooksForList returns books for any list type (SmartList, IdList, ReadingList).
func (b *SQLiteBackend) GetBooksForList(list *library.ComicListItem) ([]*library.ComicBook, error) {
	tempLib, err := b.evaluationLibrary(list)
	if err != nil {
		return nil, err
	}
	return tempLib.GetBooksForList(list)
}

// evaluationLibrary builds the library.ComicLibrary snapshot used to
// evaluate list. When list is a smart list with no BaseListId and its
// entire matcher tree translates to a safe SQL predicate (see
// matcher_sql.go), only the (superset) matching rows are fetched from
// SQLite instead of every book - the caller (library.ComicLibrary's
// MatchBooks/GetBooksForList) still re-validates every candidate against
// the exact in-memory matcher, so this can only reduce cost, never
// correctness (see comic-server-770). Otherwise falls back to
// cachedLibrary, the shared full-library snapshot (comic-server-ea5).
func (b *SQLiteBackend) evaluationLibrary(list *library.ComicListItem) (*library.ComicLibrary, error) {
	if list != nil && list.BaseListId == "" && strings.Contains(list.Type, "SmartList") {
		if pred, ok := translateMatchers(list.MatcherMode, list.Matchers); ok {
			books, err := b.db.GetBooksWhere(pred.where, pred.args...)
			if err != nil {
				return nil, err
			}
			tempLib := &library.ComicLibrary{Books: books}
			tempLib.SetCVData(b.snapshotCVData())
			return tempLib, nil
		}
	}
	return b.cachedLibrary()
}

// cachedLibrary returns a library.ComicLibrary snapshot backed by a shared
// Books/ComicLists slice pair (comic-server-ea5), built at most once per
// invalidation instead of once per call: a fast RLock path reuses the
// existing snapshot if one's already built; a cache miss escalates to the
// write lock, double-checking (another goroutine may have built it while
// this one waited) before paying the full GetAllBooks()+GetAllLists()
// cost. Includes ComicLists (not just Books) so BaseListId scoping
// resolves via FindListByID the same way it does on XMLBackend - see
// comic-server-hha.
//
// Every call gets its OWN *library.ComicLibrary wrapper around the shared
// slices, with cvData set on that wrapper alone: ComicLibrary.SetCVData
// just assigns an unexported field with no synchronization of its own, so
// mutating the SAME shared struct's cvData from multiple concurrent RLock
// callers (evaluationLibrary can run several list evaluations in
// parallel) would be a real data race. The slices themselves are safe to
// share read-only - nothing ever mutates a cached Book/ComicListItem in
// place.
func (b *SQLiteBackend) cachedLibrary() (*library.ComicLibrary, error) {
	b.mu.RLock()
	if b.libCache != nil {
		books, lists := b.libCache.Books, b.libCache.ComicLists
		b.mu.RUnlock()
		lib := &library.ComicLibrary{Books: books, ComicLists: lists}
		lib.SetCVData(b.snapshotCVData())
		return lib, nil
	}
	b.mu.RUnlock()

	b.mu.Lock()
	if b.libCache == nil {
		books, err := b.db.GetAllBooks()
		if err != nil {
			b.mu.Unlock()
			return nil, err
		}
		lists, err := b.db.GetAllLists()
		if err != nil {
			b.mu.Unlock()
			return nil, err
		}
		b.libCache = &library.ComicLibrary{Books: books, ComicLists: lists}
	}
	books, lists := b.libCache.Books, b.libCache.ComicLists
	b.mu.Unlock()

	lib := &library.ComicLibrary{Books: books, ComicLists: lists}
	lib.SetCVData(b.snapshotCVData())
	return lib, nil
}

// snapshotCVData safely reads the current cvData under RLock, for a caller
// that isn't already holding b.mu (e.g. the SQL-fast-path branch of
// evaluationLibrary, which never touches libCache at all).
func (b *SQLiteBackend) snapshotCVData() map[string]*library.CVCompleteness {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cvData
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
	b.libCache = nil

	return b.db.UpdateBookFields(book)
}

// UpdateBooks updates multiple books in the database.
func (b *SQLiteBackend) UpdateBooks(books []*library.ComicBook) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.libCache = nil

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
