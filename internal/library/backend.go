// Package library provides comic library management functionality.
package library

// Backend defines the interface for accessing comic library data.
// This abstraction allows switching between XML (in-memory) and SQLite backends.
type Backend interface {
	// Book operations
	GetBook(id string) (*ComicBook, error)
	GetAllBooks() ([]ComicBook, error)

	// List operations
	FindListByID(id string) (*ComicListItem, error)
	FindList(name string) (*ComicListItem, error)
	GetAllLists() ([]ComicListItem, error)

	// Smart list evaluation
	MatchBooks(list *ComicListItem) ([]*ComicBook, error)

	// GetBooksForList returns books for any list type (SmartList, IdList, ReadingList).
	GetBooksForList(list *ComicListItem) ([]*ComicBook, error)

	// Write operations (for reverse sync)
	UpdateBook(book *ComicBook) error
	UpdateBooks(books []*ComicBook) error

	// Lifecycle
	MarkDirty(bookID string)
	MarkManyDirty(bookIDs []string)
	Flush() error
	Close() error

	// Metadata
	LibraryID() string
	LibraryName() string
	BookCount() int

	// CanPersist returns true if changes can be saved to persistent storage.
	// Returns false if the backend is read-only or no storage path is configured.
	CanPersist() bool
}
