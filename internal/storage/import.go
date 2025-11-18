package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ImportStats tracks the results of an import operation.
type ImportStats struct {
	BooksAdded     int
	BooksUpdated   int
	BooksDeleted   int
	BooksUnchanged int

	ListsAdded     int
	ListsUpdated   int
	ListsDeleted   int
	ListsUnchanged int

	Duration time.Duration
}

// ImportOptions configures the import behavior.
type ImportOptions struct {
	DryRun  bool // If true, don't modify database
	Verbose bool // If true, log each operation
}

// Import imports a ComicRack library into the database.
// The import is idempotent - importing the same library twice results in no changes.
func (db *DB) Import(lib *library.ComicLibrary, opts ImportOptions) (*ImportStats, error) {
	start := time.Now()
	stats := &ImportStats{}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Import books
	if err := db.importBooks(tx, lib.Books, stats, opts); err != nil {
		return nil, fmt.Errorf("import books: %w", err)
	}

	// Import lists
	if err := db.importLists(tx, lib.ComicLists, "", stats, opts); err != nil {
		return nil, fmt.Errorf("import lists: %w", err)
	}

	// Store library metadata
	if !opts.DryRun {
		if err := db.storeLibraryMetadata(tx, lib); err != nil {
			return nil, fmt.Errorf("store library metadata: %w", err)
		}
	}

	// Commit transaction
	if !opts.DryRun {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit transaction: %w", err)
		}
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

func (db *DB) importBooks(tx *sql.Tx, books []library.ComicBook, stats *ImportStats, opts ImportOptions) error {
	// Get existing book IDs and hashes
	existing := make(map[string]string) // id -> hash
	rows, err := tx.Query("SELECT id, import_hash FROM books")
	if err != nil {
		return fmt.Errorf("query existing books: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return fmt.Errorf("scan book: %w", err)
		}
		existing[id] = hash
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate books: %w", err)
	}

	// Track which IDs are in the import
	imported := make(map[string]bool)

	// Process each book
	for i := range books {
		book := &books[i]
		imported[book.ID] = true

		hash := computeBookHash(book)
		existingHash, exists := existing[book.ID]

		if !exists {
			// New book - insert
			if !opts.DryRun {
				if err := db.insertBook(tx, book, hash); err != nil {
					return fmt.Errorf("insert book %s: %w", book.ID, err)
				}
			}
			stats.BooksAdded++
		} else if hash != existingHash {
			// Changed book - update
			if !opts.DryRun {
				if err := db.updateBook(tx, book, hash); err != nil {
					return fmt.Errorf("update book %s: %w", book.ID, err)
				}
			}
			stats.BooksUpdated++
		} else {
			// Unchanged
			stats.BooksUnchanged++
		}
	}

	// Delete books not in import
	for id := range existing {
		if !imported[id] {
			if !opts.DryRun {
				if _, err := tx.Exec("DELETE FROM books WHERE id = ?", id); err != nil {
					return fmt.Errorf("delete book %s: %w", id, err)
				}
			}
			stats.BooksDeleted++
		}
	}

	return nil
}

func (db *DB) insertBook(tx *sql.Tx, book *library.ComicBook, hash string) error {
	// Convert pages to JSON
	pagesJSON, err := json.Marshal(book.Pages)
	if err != nil {
		return fmt.Errorf("marshal pages: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.Exec(`
		INSERT INTO books (
			id, file_path, title, series, number, volume, year, month, day,
			publisher, imprint, genre, format, age_rating, language_iso,
			summary, notes, review, story_arc, series_group,
			alternate_series, alternate_number, alternate_count, count,
			writer, penciller, inker, colorist, letterer, cover_artist, editor, translator,
			characters, teams, locations, main_character_or_team,
			current_page, last_page, last_page_read, open_count, opened_time,
			rating, community_rating,
			checked, file_is_missing, comic_info_is_dirty,
			page_count, web, scan_information, series_complete,
			black_and_white, manga,
			preferred_front_cover, added_time, released_time,
			enable_proposed, enable_dynamic_update, last_opened_from_list_id,
			pages, import_hash, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?,
			?, ?, ?,
			?, ?, ?,
			?, ?, ?
		)
	`,
		book.ID, book.FilePath, book.Title, book.Series, book.Number, book.Volume, book.Year, book.Month, book.Day,
		book.Publisher, book.Imprint, book.Genre, book.Format, book.AgeRating, book.LanguageISO,
		book.Summary, book.Notes, book.Review, book.StoryArc, book.SeriesGroup,
		book.AlternateSeries, book.AlternateNumber, book.AlternateCount, book.Count,
		book.Writer, book.Penciller, book.Inker, book.Colorist, book.Letterer, book.CoverArtist, book.Editor, book.Translator,
		book.Characters, book.Teams, book.Locations, book.MainCharacterOrTeam,
		book.CurrentPage, book.LastPage, book.LastPageRead, book.OpenCount, formatTime(book.OpenedTime),
		book.Rating, book.CommunityRating,
		boolToInt(book.Checked), boolToInt(book.FileIsMissing), boolToInt(book.ComicInfoIsDirty),
		book.PageCount, book.Web, book.ScanInformation, book.SeriesComplete,
		book.BlackAndWhite, book.Manga,
		book.PreferredFrontCover, formatTime(book.AddedTime), formatTime(book.ReleasedTime),
		boolToInt(book.EnableProposed), boolToInt(book.EnableDynamicUpdate), book.LastOpenedFromListID,
		string(pagesJSON), hash, now,
	)
	if err != nil {
		return err
	}

	// Insert custom values
	if err := db.insertBookCustomValues(tx, book); err != nil {
		return fmt.Errorf("insert custom values: %w", err)
	}

	// Insert tags
	if err := db.insertBookTags(tx, book); err != nil {
		return fmt.Errorf("insert tags: %w", err)
	}

	return nil
}

func (db *DB) updateBook(tx *sql.Tx, book *library.ComicBook, hash string) error {
	// Convert pages to JSON
	pagesJSON, err := json.Marshal(book.Pages)
	if err != nil {
		return fmt.Errorf("marshal pages: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.Exec(`
		UPDATE books SET
			file_path = ?, title = ?, series = ?, number = ?, volume = ?, year = ?, month = ?, day = ?,
			publisher = ?, imprint = ?, genre = ?, format = ?, age_rating = ?, language_iso = ?,
			summary = ?, notes = ?, review = ?, story_arc = ?, series_group = ?,
			alternate_series = ?, alternate_number = ?, alternate_count = ?, count = ?,
			writer = ?, penciller = ?, inker = ?, colorist = ?, letterer = ?, cover_artist = ?, editor = ?, translator = ?,
			characters = ?, teams = ?, locations = ?, main_character_or_team = ?,
			current_page = ?, last_page = ?, last_page_read = ?, open_count = ?, opened_time = ?,
			rating = ?, community_rating = ?,
			checked = ?, file_is_missing = ?, comic_info_is_dirty = ?,
			page_count = ?, web = ?, scan_information = ?, series_complete = ?,
			black_and_white = ?, manga = ?,
			preferred_front_cover = ?, added_time = ?, released_time = ?,
			enable_proposed = ?, enable_dynamic_update = ?, last_opened_from_list_id = ?,
			pages = ?, import_hash = ?, updated_at = ?
		WHERE id = ?
	`,
		book.FilePath, book.Title, book.Series, book.Number, book.Volume, book.Year, book.Month, book.Day,
		book.Publisher, book.Imprint, book.Genre, book.Format, book.AgeRating, book.LanguageISO,
		book.Summary, book.Notes, book.Review, book.StoryArc, book.SeriesGroup,
		book.AlternateSeries, book.AlternateNumber, book.AlternateCount, book.Count,
		book.Writer, book.Penciller, book.Inker, book.Colorist, book.Letterer, book.CoverArtist, book.Editor, book.Translator,
		book.Characters, book.Teams, book.Locations, book.MainCharacterOrTeam,
		book.CurrentPage, book.LastPage, book.LastPageRead, book.OpenCount, formatTime(book.OpenedTime),
		book.Rating, book.CommunityRating,
		boolToInt(book.Checked), boolToInt(book.FileIsMissing), boolToInt(book.ComicInfoIsDirty),
		book.PageCount, book.Web, book.ScanInformation, book.SeriesComplete,
		book.BlackAndWhite, book.Manga,
		book.PreferredFrontCover, formatTime(book.AddedTime), formatTime(book.ReleasedTime),
		boolToInt(book.EnableProposed), boolToInt(book.EnableDynamicUpdate), book.LastOpenedFromListID,
		string(pagesJSON), hash, now,
		book.ID,
	)
	if err != nil {
		return err
	}

	// Delete and re-insert custom values
	if _, err := tx.Exec("DELETE FROM book_custom_values WHERE book_id = ?", book.ID); err != nil {
		return fmt.Errorf("delete custom values: %w", err)
	}
	if err := db.insertBookCustomValues(tx, book); err != nil {
		return fmt.Errorf("insert custom values: %w", err)
	}

	// Delete and re-insert tags
	if _, err := tx.Exec("DELETE FROM book_tags WHERE book_id = ?", book.ID); err != nil {
		return fmt.Errorf("delete tags: %w", err)
	}
	if err := db.insertBookTags(tx, book); err != nil {
		return fmt.Errorf("insert tags: %w", err)
	}

	return nil
}

func (db *DB) insertBookCustomValues(tx *sql.Tx, book *library.ComicBook) error {
	if book.CustomValuesStore == "" {
		return nil
	}

	// Parse custom values from ComicRack format: ",key1=value1,key2=value2"
	customValues := parseCustomValues(book.CustomValuesStore)

	for key, value := range customValues {
		_, err := tx.Exec(
			"INSERT INTO book_custom_values (book_id, key, value) VALUES (?, ?, ?)",
			book.ID, key, value,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) insertBookTags(tx *sql.Tx, book *library.ComicBook) error {
	if book.Tags == "" {
		return nil
	}

	// Tags are comma-separated
	tags := strings.Split(book.Tags, ",")
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		_, err := tx.Exec(
			"INSERT INTO book_tags (book_id, tag) VALUES (?, ?)",
			book.ID, tag,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) importLists(tx *sql.Tx, lists []library.ComicListItem, parentID string, stats *ImportStats, opts ImportOptions) error {
	// Get existing lists and hashes
	existing := make(map[string]string) // id -> hash
	rows, err := tx.Query("SELECT id, import_hash FROM lists")
	if err != nil {
		return fmt.Errorf("query existing lists: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return fmt.Errorf("scan list: %w", err)
		}
		existing[id] = hash
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate lists: %w", err)
	}

	// Track which IDs are in the import
	imported := make(map[string]bool)

	// Process lists recursively
	if err := db.importListsRecursive(tx, lists, parentID, existing, imported, stats, opts); err != nil {
		return err
	}

	// Delete lists not in import (only on first call, when parentID is empty)
	if parentID == "" {
		for id := range existing {
			if !imported[id] {
				if !opts.DryRun {
					if _, err := tx.Exec("DELETE FROM lists WHERE id = ?", id); err != nil {
						return fmt.Errorf("delete list %s: %w", id, err)
					}
				}
				stats.ListsDeleted++
			}
		}
	}

	return nil
}

func (db *DB) importListsRecursive(tx *sql.Tx, lists []library.ComicListItem, parentID string, existing map[string]string, imported map[string]bool, stats *ImportStats, opts ImportOptions) error {
	for i := range lists {
		list := &lists[i]
		imported[list.ID] = true

		hash := computeListHash(list)
		existingHash, exists := existing[list.ID]

		if !exists {
			// New list - insert
			if !opts.DryRun {
				if err := db.insertList(tx, list, parentID, hash); err != nil {
					return fmt.Errorf("insert list %s: %w", list.ID, err)
				}
			}
			stats.ListsAdded++
		} else if hash != existingHash {
			// Changed list - update
			if !opts.DryRun {
				if err := db.updateList(tx, list, parentID, hash); err != nil {
					return fmt.Errorf("update list %s: %w", list.ID, err)
				}
			}
			stats.ListsUpdated++
		} else {
			// Unchanged
			stats.ListsUnchanged++
		}

		// Process child lists (for folders)
		if len(list.ChildItems) > 0 {
			if err := db.importListsRecursive(tx, list.ChildItems, list.ID, existing, imported, stats, opts); err != nil {
				return err
			}
		}
	}

	return nil
}

func (db *DB) insertList(tx *sql.Tx, list *library.ComicListItem, parentID string, hash string) error {
	// Convert matchers to JSON
	matchersJSON, err := json.Marshal(list.Matchers)
	if err != nil {
		return fmt.Errorf("marshal matchers: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	var parentIDValue interface{}
	if parentID != "" {
		parentIDValue = parentID
	}

	_, err = tx.Exec(`
		INSERT INTO lists (
			id, name, type, parent_id, description, favorite, collapsed,
			matcher_mode, matchers, book_count, import_hash, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		list.ID, list.Name, list.Type, parentIDValue, list.Description,
		boolToInt(list.Favorite), boolToInt(list.Collapsed),
		list.MatcherMode, string(matchersJSON), list.BookCount, hash, now,
	)
	if err != nil {
		return err
	}

	// Insert reading list items if this is a reading list
	if list.Type == "ComicReadingList" && len(list.Items) > 0 {
		for i, item := range list.Items {
			_, err := tx.Exec(
				"INSERT INTO reading_list_items (list_id, book_id, position) VALUES (?, ?, ?)",
				list.ID, item.ID, i,
			)
			if err != nil {
				return fmt.Errorf("insert reading list item: %w", err)
			}
		}
	}

	return nil
}

func (db *DB) updateList(tx *sql.Tx, list *library.ComicListItem, parentID string, hash string) error {
	// Convert matchers to JSON
	matchersJSON, err := json.Marshal(list.Matchers)
	if err != nil {
		return fmt.Errorf("marshal matchers: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	var parentIDValue interface{}
	if parentID != "" {
		parentIDValue = parentID
	}

	_, err = tx.Exec(`
		UPDATE lists SET
			name = ?, type = ?, parent_id = ?, description = ?, favorite = ?, collapsed = ?,
			matcher_mode = ?, matchers = ?, book_count = ?, import_hash = ?, updated_at = ?
		WHERE id = ?
	`,
		list.Name, list.Type, parentIDValue, list.Description,
		boolToInt(list.Favorite), boolToInt(list.Collapsed),
		list.MatcherMode, string(matchersJSON), list.BookCount, hash, now,
		list.ID,
	)
	if err != nil {
		return err
	}

	// Delete and re-insert reading list items
	if _, err := tx.Exec("DELETE FROM reading_list_items WHERE list_id = ?", list.ID); err != nil {
		return fmt.Errorf("delete reading list items: %w", err)
	}

	if list.Type == "ComicReadingList" && len(list.Items) > 0 {
		for i, item := range list.Items {
			_, err := tx.Exec(
				"INSERT INTO reading_list_items (list_id, book_id, position) VALUES (?, ?, ?)",
				list.ID, item.ID, i,
			)
			if err != nil {
				return fmt.Errorf("insert reading list item: %w", err)
			}
		}
	}

	return nil
}

func (db *DB) storeLibraryMetadata(tx *sql.Tx, lib *library.ComicLibrary) error {
	metadata := map[string]string{
		"library_id":   lib.ID,
		"library_name": lib.Name,
		"imported_at":  time.Now().UTC().Format(time.RFC3339),
	}

	for key, value := range metadata {
		_, err := tx.Exec(`
			INSERT INTO library_metadata (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, key, value)
		if err != nil {
			return fmt.Errorf("store metadata %s: %w", key, err)
		}
	}

	return nil
}

// computeBookHash creates a hash of all book fields for change detection.
func computeBookHash(book *library.ComicBook) string {
	h := sha256.New()

	// Hash all fields that we care about for change detection
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%d|%d|%d|%d|",
		book.ID, book.FilePath, book.Title, book.Series, book.Number,
		book.Volume, book.Year, book.Month, book.Day)
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|",
		book.Publisher, book.Imprint, book.Genre, book.Format, book.AgeRating, book.LanguageISO)
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|",
		book.Summary, book.Notes, book.Review, book.StoryArc, book.SeriesGroup)
	fmt.Fprintf(h, "%s|%s|%d|%d|",
		book.AlternateSeries, book.AlternateNumber, book.AlternateCount, book.Count)
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|",
		book.Writer, book.Penciller, book.Inker, book.Colorist, book.Letterer, book.CoverArtist, book.Editor)
	fmt.Fprintf(h, "%s|%s|%s|%s|",
		book.Characters, book.Teams, book.Locations, book.MainCharacterOrTeam)
	fmt.Fprintf(h, "%d|%d|%d|%d|%v|",
		book.CurrentPage, book.LastPage, book.LastPageRead, book.OpenCount, book.OpenedTime.Time)
	fmt.Fprintf(h, "%f|%f|%t|%t|%t|",
		book.Rating, book.CommunityRating, book.Checked, book.FileIsMissing, book.ComicInfoIsDirty)
	fmt.Fprintf(h, "%d|%s|%s|%s|%d|%v|%v|",
		book.PageCount, book.Web, book.ScanInformation, book.SeriesComplete,
		book.PreferredFrontCover, book.AddedTime.Time, book.ReleasedTime.Time)
	fmt.Fprintf(h, "%t|%t|%s|%s|%s|",
		book.EnableProposed, book.EnableDynamicUpdate, book.LastOpenedFromListID,
		book.Tags, book.CustomValuesStore)

	// Hash pages
	for _, page := range book.Pages {
		fmt.Fprintf(h, "%d|%s|%d|%d|%s|%d|%s|",
			page.Image, page.Type, page.ImageWidth, page.ImageHeight, page.Bookmark, page.ImageSize, page.Key)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// computeListHash creates a hash of all list fields for change detection.
func computeListHash(list *library.ComicListItem) string {
	h := sha256.New()

	fmt.Fprintf(h, "%s|%s|%s|%s|%t|%t|%s|%d|",
		list.ID, list.Name, list.Type, list.Description,
		list.Favorite, list.Collapsed, list.MatcherMode, list.BookCount)

	// Hash matchers
	matchersJSON, _ := json.Marshal(list.Matchers)
	h.Write(matchersJSON)

	// Hash reading list items
	for _, item := range list.Items {
		fmt.Fprintf(h, "%s|", item.ID)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// parseCustomValues parses the ComicRack custom values format.
// Format: ",key1=value1,key2=value2"
func parseCustomValues(store string) map[string]string {
	result := make(map[string]string)
	if store == "" {
		return result
	}

	// Split by comma, handling the leading comma
	parts := strings.Split(store, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "="); idx > 0 {
			key := part[:idx]
			value := part[idx+1:]
			result[key] = value
		}
	}

	return result
}

func formatTime(t library.ComicTime) string {
	if t.Time.IsZero() {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
