package sync

import (
	"fmt"
	"math/rand"
	"os"
	"sort"

	"github.com/duckpuppy/comic-server/internal/library"
)

// LimitType represents the unit for limiting books (count, MB, or GB)
type LimitType int

const (
	LimitTypeBooks LimitType = iota // Default: limit by book count
	LimitTypeMB                     // Limit by megabytes
	LimitTypeGB                     // Limit by gigabytes
)

// String returns the string representation of a LimitType
func (lt LimitType) String() string {
	switch lt {
	case LimitTypeBooks:
		return "books"
	case LimitTypeMB:
		return "mb"
	case LimitTypeGB:
		return "gb"
	default:
		return "unknown"
	}
}

// MarshalText implements encoding.TextMarshaler
func (lt LimitType) MarshalText() ([]byte, error) {
	return []byte(lt.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (lt *LimitType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "books":
		*lt = LimitTypeBooks
	case "mb":
		*lt = LimitTypeMB
	case "gb":
		*lt = LimitTypeGB
	default:
		return fmt.Errorf("unknown limit type: %s", text)
	}
	return nil
}

// SortType represents how to sort books before limiting
type SortType int

const (
	SortTypeSeries          SortType = iota // Group by series, sort by volume/number
	SortTypeRandom                          // Random shuffling
	SortTypePublished                       // By publication date (Year/Month/Day)
	SortTypeAdded                           // By date added to library
	SortTypeStoryArc                        // Group by story arc
	SortTypeListOrder                       // Preserve original smart list order
	SortTypeAlternateSeries                 // By alternate series field
)

// String returns the string representation of a SortType
func (st SortType) String() string {
	switch st {
	case SortTypeSeries:
		return "series"
	case SortTypeRandom:
		return "random"
	case SortTypePublished:
		return "published"
	case SortTypeAdded:
		return "added"
	case SortTypeStoryArc:
		return "story-arc"
	case SortTypeListOrder:
		return "list-order"
	case SortTypeAlternateSeries:
		return "alternate-series"
	default:
		return "unknown"
	}
}

// MarshalText implements encoding.TextMarshaler
func (st SortType) MarshalText() ([]byte, error) {
	return []byte(st.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (st *SortType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "series":
		*st = SortTypeSeries
	case "random":
		*st = SortTypeRandom
	case "published":
		*st = SortTypePublished
	case "added":
		*st = SortTypeAdded
	case "story-arc":
		*st = SortTypeStoryArc
	case "list-order":
		*st = SortTypeListOrder
	case "alternate-series":
		*st = SortTypeAlternateSeries
	default:
		return fmt.Errorf("unknown sort type: %s", text)
	}
	return nil
}

// SharedListSettings contains per-list sync options
// These control how books are filtered, sorted, and limited when syncing
type SharedListSettings struct {
	// Filtering options
	OnlyUnread   bool `yaml:"only_unread" toml:"only_unread"`       // Only sync unread books (CurrentPage < PageCount)
	KeepLastRead bool `yaml:"keep_last_read" toml:"keep_last_read"` // When OnlyUnread=true, include N recently read books for context
	OnlyChecked  bool `yaml:"only_checked" toml:"only_checked"`     // Only sync books marked as "checked"

	// Limiting options
	Limit          bool      `yaml:"limit" toml:"limit"`                       // Enable limiting
	LimitValue     int       `yaml:"limit_value" toml:"limit_value"`           // Limit amount (default 50)
	LimitValueType LimitType `yaml:"limit_value_type" toml:"limit_value_type"` // Limit unit: Books, MB, or GB

	// Sorting options
	Sort         bool     `yaml:"sort" toml:"sort"`                     // Enable sorting
	ListSortType SortType `yaml:"list_sort_type" toml:"list_sort_type"` // How to sort (default: Series)
}

// DefaultSettings returns the default SharedListSettings
func DefaultSettings() *SharedListSettings {
	return &SharedListSettings{
		OnlyUnread:     false,
		KeepLastRead:   false,
		OnlyChecked:    false,
		Limit:          false,
		LimitValue:     50,
		LimitValueType: LimitTypeBooks,
		Sort:           true,
		ListSortType:   SortTypeSeries,
	}
}

// ApplySettings applies all configured options to a list of books
// Returns the filtered, sorted, and limited list
func ApplySettings(books []*library.ComicBook, settings *SharedListSettings) ([]*library.ComicBook, error) {
	if settings == nil {
		settings = DefaultSettings()
	}

	result := books

	// Step 1: Apply OnlyChecked filter
	if settings.OnlyChecked {
		result = filterOnlyChecked(result)
	}

	// Step 2: Apply OnlyUnread filter
	if settings.OnlyUnread {
		result = filterOnlyUnread(result)

		// Step 3: Add back last read books if KeepLastRead is enabled
		if settings.KeepLastRead {
			// Get the read books we filtered out
			readBooks := filterOnlyRead(books)
			if settings.OnlyChecked {
				readBooks = filterOnlyChecked(readBooks)
			}

			// Add back the most recently opened books
			// TODO: Make this configurable (default 3 from ComicRackCE)
			keepCount := 3
			lastRead := getMostRecentlyOpened(readBooks, keepCount)
			result = append(result, lastRead...)
		}
	}

	// Step 4: Apply sorting
	if settings.Sort {
		result = sortBooks(result, settings.ListSortType)
	}

	// Step 5: Apply limit
	if settings.Limit {
		var err error
		result, err = limitBooks(result, settings.LimitValue, settings.LimitValueType)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// filterOnlyChecked returns only books marked as checked
func filterOnlyChecked(books []*library.ComicBook) []*library.ComicBook {
	var result []*library.ComicBook
	for _, book := range books {
		if book.Checked {
			result = append(result, book)
		}
	}
	return result
}

// filterOnlyUnread returns only unread books (CurrentPage < PageCount)
func filterOnlyUnread(books []*library.ComicBook) []*library.ComicBook {
	var result []*library.ComicBook
	for _, book := range books {
		// Book is unread if CurrentPage < PageCount or CurrentPage == 0
		// Handle case where PageCount might be 0 (no pages tracked)
		if book.PageCount > 0 && book.CurrentPage < book.PageCount {
			result = append(result, book)
		} else if book.PageCount == 0 && book.CurrentPage == 0 {
			// If no page tracking, consider it unread
			result = append(result, book)
		}
	}
	return result
}

// filterOnlyRead returns only read books (CurrentPage >= PageCount)
func filterOnlyRead(books []*library.ComicBook) []*library.ComicBook {
	var result []*library.ComicBook
	for _, book := range books {
		if book.PageCount > 0 && book.CurrentPage >= book.PageCount {
			result = append(result, book)
		}
	}
	return result
}

// getMostRecentlyOpened returns the N most recently opened books
func getMostRecentlyOpened(books []*library.ComicBook, n int) []*library.ComicBook {
	if len(books) == 0 || n <= 0 {
		return nil
	}

	// Sort by OpenedTime descending (most recent first)
	sorted := make([]*library.ComicBook, len(books))
	copy(sorted, books)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OpenedTime.Time.After(sorted[j].OpenedTime.Time)
	})

	// Return first N books
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// sortBooks sorts books according to the specified SortType
func sortBooks(books []*library.ComicBook, sortType SortType) []*library.ComicBook {
	if len(books) <= 1 {
		return books
	}

	// Make a copy to avoid modifying the original slice
	result := make([]*library.ComicBook, len(books))
	copy(result, books)

	switch sortType {
	case SortTypeSeries:
		sortBySeries(result)
	case SortTypeRandom:
		sortRandom(result)
	case SortTypePublished:
		sortByPublished(result)
	case SortTypeAdded:
		sortByAdded(result)
	case SortTypeStoryArc:
		sortByStoryArc(result)
	case SortTypeAlternateSeries:
		sortByAlternateSeries(result)
	case SortTypeListOrder:
		// No sorting - preserve original order
	}

	return result
}

// sortBySeries sorts by Series, then Volume, then Number
func sortBySeries(books []*library.ComicBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].Series != books[j].Series {
			return books[i].Series < books[j].Series
		}
		if books[i].Volume != books[j].Volume {
			return books[i].Volume < books[j].Volume
		}
		// Compare numbers as strings (they may contain decimals like "1.5")
		return books[i].Number < books[j].Number
	})
}

// sortRandom shuffles books randomly
func sortRandom(books []*library.ComicBook) {
	rand.Shuffle(len(books), func(i, j int) {
		books[i], books[j] = books[j], books[i]
	})
}

// sortByPublished sorts by publication date (Year, Month, Day)
func sortByPublished(books []*library.ComicBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].Year != books[j].Year {
			return books[i].Year < books[j].Year
		}
		if books[i].Month != books[j].Month {
			return books[i].Month < books[j].Month
		}
		return books[i].Day < books[j].Day
	})
}

// sortByAdded sorts by date added to library
func sortByAdded(books []*library.ComicBook) {
	sort.Slice(books, func(i, j int) bool {
		return books[i].AddedTime.Time.Before(books[j].AddedTime.Time)
	})
}

// sortByStoryArc sorts by StoryArc field, then by published date within arc
func sortByStoryArc(books []*library.ComicBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].StoryArc != books[j].StoryArc {
			return books[i].StoryArc < books[j].StoryArc
		}
		// Within same story arc, sort by published date
		if books[i].Year != books[j].Year {
			return books[i].Year < books[j].Year
		}
		if books[i].Month != books[j].Month {
			return books[i].Month < books[j].Month
		}
		return books[i].Day < books[j].Day
	})
}

// sortByAlternateSeries sorts by AlternateSeries field
func sortByAlternateSeries(books []*library.ComicBook) {
	sort.Slice(books, func(i, j int) bool {
		if books[i].AlternateSeries != books[j].AlternateSeries {
			return books[i].AlternateSeries < books[j].AlternateSeries
		}
		if books[i].AlternateCount != books[j].AlternateCount {
			return books[i].AlternateCount < books[j].AlternateCount
		}
		return books[i].AlternateNumber < books[j].AlternateNumber
	})
}

// limitBooks limits the book list by count or size
func limitBooks(books []*library.ComicBook, limitValue int, limitType LimitType) ([]*library.ComicBook, error) {
	if limitValue <= 0 {
		return books, nil
	}

	if len(books) == 0 {
		return books, nil
	}

	switch limitType {
	case LimitTypeBooks:
		return limitByCount(books, limitValue), nil
	case LimitTypeMB:
		return limitBySize(books, int64(limitValue)*1024*1024)
	case LimitTypeGB:
		return limitBySize(books, int64(limitValue)*1024*1024*1024)
	default:
		return nil, fmt.Errorf("unknown limit type: %v", limitType)
	}
}

// limitByCount returns the first N books
func limitByCount(books []*library.ComicBook, count int) []*library.ComicBook {
	if count >= len(books) {
		return books
	}
	return books[:count]
}

// limitBySize returns books up to the specified total size in bytes
func limitBySize(books []*library.ComicBook, maxBytes int64) ([]*library.ComicBook, error) {
	var result []*library.ComicBook
	var totalSize int64

	for _, book := range books {
		// Get file size for this book
		size, err := getBookFileSize(book)
		if err != nil {
			// If we can't get size, skip this book
			continue
		}

		// Check if adding this book would exceed limit
		if totalSize+size > maxBytes && len(result) > 0 {
			// Stop here - we've reached the limit
			break
		}

		result = append(result, book)
		totalSize += size
	}

	return result, nil
}

// getBookFileSize returns the file size in bytes for a comic book
func getBookFileSize(book *library.ComicBook) (int64, error) {
	if book.FilePath == "" {
		return 0, fmt.Errorf("book %s has no file path", book.ID)
	}

	fileInfo, err := os.Stat(book.FilePath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat %s: %w", book.FilePath, err)
	}

	return fileInfo.Size(), nil
}
