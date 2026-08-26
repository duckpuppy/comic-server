package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// booksSelectColumns is the column list shared by every query that scans a
// full ComicBook row via scanBook/scanBookFromRows.
const booksSelectColumns = `
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
	file_size, file_modified_time, file_creation_time,
	isbn, book_age, book_condition, book_store, book_owner,
	book_collection_status, book_notes, book_location,
	book_price, new_pages,
	enable_proposed, enable_dynamic_update, last_opened_from_list_id,
	pages
`

// GetBook retrieves a single book by ID.
func (db *DB) GetBook(id string) (*library.ComicBook, error) {
	row := db.QueryRow("SELECT "+booksSelectColumns+" FROM books WHERE id = ? AND deleted_at IS NULL", id)

	book, err := scanBook(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book %s: %w", id, err)
	}

	// Load custom values
	if err := db.loadBookCustomValues(book); err != nil {
		return nil, err
	}

	// Load tags
	if err := db.loadBookTags(book); err != nil {
		return nil, err
	}

	return book, nil
}

// GetAllBooks retrieves all books from the database.
func (db *DB) GetAllBooks() ([]library.ComicBook, error) {
	return db.queryBooks("")
}

// GetBooksWhere retrieves books matching a raw SQL WHERE clause (no "WHERE"
// keyword - pass "" for no filter). Used by SQLiteBackend.MatchBooks to
// narrow the row fetch via a translated smart-list matcher predicate (see
// internal/storage/matcher_sql.go) instead of always loading every book -
// see comic-server-770. whereClause must only reference columns on the
// books table; args are passed positionally to the underlying query.
func (db *DB) GetBooksWhere(whereClause string, args ...any) ([]library.ComicBook, error) {
	return db.queryBooks(whereClause, args...)
}

// queryBooks runs a SELECT over the books table (optionally filtered by
// whereClause, always excluding soft-deleted rows - see comic-server-b53)
// and batch-loads tags/custom values for the result set in a small, fixed
// number of queries rather than two extra round trips per book - at
// real-library scale (67K+ books) the latter was the dominant cost of a
// cold GetAllBooks(), not the Go-side matcher evaluation that follows it
// (see comic-server-770 / comic-server-cg1).
func (db *DB) queryBooks(whereClause string, args ...any) ([]library.ComicBook, error) {
	query := "SELECT " + booksSelectColumns + " FROM books WHERE deleted_at IS NULL"
	if whereClause != "" {
		query += " AND (" + whereClause + ")"
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query books: %w", err)
	}
	defer rows.Close()

	var books []library.ComicBook
	for rows.Next() {
		book, err := scanBookFromRows(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate books: %w", err)
	}

	if err := db.loadTagsAndCustomValuesBatch(books); err != nil {
		return nil, err
	}

	return books, nil
}

// batchLoadChunkSize bounds how many book IDs go into a single IN (...)
// clause in loadTagsAndCustomValuesBatch, to stay well under SQLite's host
// parameter limit (SQLITE_MAX_VARIABLE_NUMBER, historically as low as 999)
// regardless of how many books are being loaded.
const batchLoadChunkSize = 500

// loadTagsAndCustomValuesBatch fills in Tags/CustomValuesStore for every
// book in books using O(len(books)/batchLoadChunkSize) queries total,
// instead of the 2 extra queries per book that GetAllBooks used to run
// (comic-server-770). books is mutated in place.
func (db *DB) loadTagsAndCustomValuesBatch(books []library.ComicBook) error {
	if len(books) == 0 {
		return nil
	}

	idIndex := make(map[string]int, len(books))
	ids := make([]string, len(books))
	for i := range books {
		idIndex[books[i].ID] = i
		ids[i] = books[i].ID
	}

	tagsByBook := make(map[string][]string)
	err := db.chunkedInQuery(ids, "SELECT book_id, tag FROM book_tags WHERE book_id IN (%s)", func(rows *sql.Rows) error {
		var bookID, tag string
		if err := rows.Scan(&bookID, &tag); err != nil {
			return err
		}
		tagsByBook[bookID] = append(tagsByBook[bookID], tag)
		return nil
	})
	if err != nil {
		return fmt.Errorf("batch load tags: %w", err)
	}
	for bookID, tags := range tagsByBook {
		if i, ok := idIndex[bookID]; ok && len(tags) > 0 {
			books[i].Tags = joinStrings(tags, ", ")
		}
	}

	cvByBook := make(map[string][]string)
	err = db.chunkedInQuery(ids, "SELECT book_id, key, value FROM book_custom_values WHERE book_id IN (%s)", func(rows *sql.Rows) error {
		var bookID, key, value string
		if err := rows.Scan(&bookID, &key, &value); err != nil {
			return err
		}
		cvByBook[bookID] = append(cvByBook[bookID], fmt.Sprintf("%s=%s", key, value))
		return nil
	})
	if err != nil {
		return fmt.Errorf("batch load custom values: %w", err)
	}
	for bookID, parts := range cvByBook {
		if i, ok := idIndex[bookID]; ok && len(parts) > 0 {
			books[i].CustomValuesStore = "," + joinStrings(parts, ",")
		}
	}

	return nil
}

// chunkedInQuery runs queryTemplate (containing exactly one %s, filled in
// with a "?,?,...?" placeholder list) against ids in batches of
// batchLoadChunkSize, calling scan for every row across every batch.
func (db *DB) chunkedInQuery(ids []string, queryTemplate string, scan func(*sql.Rows) error) error {
	for start := 0; start < len(ids); start += batchLoadChunkSize {
		end := start + batchLoadChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}
		query := fmt.Sprintf(queryTemplate, strings.Join(placeholders, ","))

		if err := func() error {
			rows, err := db.Query(query, args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				if err := scan(rows); err != nil {
					return err
				}
			}
			return rows.Err()
		}(); err != nil {
			return err
		}
	}
	return nil
}

// GetBookCount returns the total number of books in the database.
func (db *DB) GetBookCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM books WHERE deleted_at IS NULL").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count books: %w", err)
	}
	return count, nil
}

// GetList retrieves a single list by ID.
func (db *DB) GetList(id string) (*library.ComicListItem, error) {
	row := db.QueryRow(`
		SELECT id, name, type, description, favorite, collapsed, matcher_mode, matchers, book_count
		FROM lists WHERE id = ? AND deleted_at IS NULL
	`, id)

	list, err := scanList(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get list %s: %w", id, err)
	}

	// Load reading list items if applicable
	if list.Type == "ComicReadingList" {
		if err := db.loadReadingListItems(list); err != nil {
			return nil, err
		}
	}

	return list, nil
}

// GetAllLists retrieves all lists from the database.
func (db *DB) GetAllLists() ([]library.ComicListItem, error) {
	rows, err := db.Query(`
		SELECT id, name, type, parent_id, description, favorite, collapsed, matcher_mode, matchers, book_count
		FROM lists
		WHERE deleted_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("query lists: %w", err)
	}
	defer rows.Close()

	// First pass: collect all lists
	listMap := make(map[string]*library.ComicListItem)
	var rootLists []string
	parentMap := make(map[string]string) // child -> parent

	for rows.Next() {
		var id, name, listType, matcherMode string
		var parentID sql.NullString
		var description sql.NullString
		var favorite, collapsed bool
		var matchersJSON string
		var bookCount int

		err := rows.Scan(&id, &name, &listType, &parentID, &description, &favorite, &collapsed, &matcherMode, &matchersJSON, &bookCount)
		if err != nil {
			return nil, fmt.Errorf("scan list: %w", err)
		}

		list := &library.ComicListItem{
			ID:          id,
			Name:        name,
			Type:        listType,
			MatcherMode: matcherMode,
			Favorite:    favorite,
			Collapsed:   collapsed,
			BookCount:   bookCount,
		}

		if description.Valid {
			list.Description = description.String
		}

		// Parse matchers
		if matchersJSON != "" && matchersJSON != "null" {
			if err := json.Unmarshal([]byte(matchersJSON), &list.Matchers); err != nil {
				return nil, fmt.Errorf("unmarshal matchers for list %s: %w", id, err)
			}
		}

		listMap[id] = list

		if parentID.Valid {
			parentMap[id] = parentID.String
		} else {
			rootLists = append(rootLists, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lists: %w", err)
	}

	// Second pass: build tree structure
	for childID, parentID := range parentMap {
		parent := listMap[parentID]
		child := listMap[childID]
		if parent != nil && child != nil {
			parent.ChildItems = append(parent.ChildItems, *child)
		}
	}

	// Load reading list items
	for _, list := range listMap {
		if list.Type == "ComicReadingList" {
			if err := db.loadReadingListItems(list); err != nil {
				return nil, err
			}
		}
	}

	// Return root lists
	var result []library.ComicListItem
	for _, id := range rootLists {
		if list := listMap[id]; list != nil {
			result = append(result, *list)
		}
	}

	return result, nil
}

// scanner interface for both sql.Row and sql.Rows
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanBook(s scanner) (*library.ComicBook, error) {
	var book library.ComicBook
	var openedTime, addedTime, releasedTime sql.NullString
	var fileModifiedTime, fileCreationTime sql.NullString
	var pagesJSON string
	var checked, fileIsMissing, comicInfoIsDirty, enableProposed, enableDynamicUpdate int

	err := s.Scan(
		&book.ID, &book.FilePath, &book.Title, &book.Series, &book.Number, &book.Volume, &book.Year, &book.Month, &book.Day,
		&book.Publisher, &book.Imprint, &book.Genre, &book.Format, &book.AgeRating, &book.LanguageISO,
		&book.Summary, &book.Notes, &book.Review, &book.StoryArc, &book.SeriesGroup,
		&book.AlternateSeries, &book.AlternateNumber, &book.AlternateCount, &book.Count,
		&book.Writer, &book.Penciller, &book.Inker, &book.Colorist, &book.Letterer, &book.CoverArtist, &book.Editor, &book.Translator,
		&book.Characters, &book.Teams, &book.Locations, &book.MainCharacterOrTeam,
		&book.CurrentPage, &book.LastPage, &book.LastPageRead, &book.OpenCount, &openedTime,
		&book.Rating, &book.CommunityRating,
		&checked, &fileIsMissing, &comicInfoIsDirty,
		&book.PageCount, &book.Web, &book.ScanInformation, &book.SeriesComplete,
		&book.BlackAndWhite, &book.Manga,
		&book.PreferredFrontCover, &addedTime, &releasedTime,
		&book.FileSize, &fileModifiedTime, &fileCreationTime,
		&book.ISBN, &book.BookAge, &book.BookCondition, &book.BookStore, &book.BookOwner,
		&book.BookCollectionStatus, &book.BookNotes, &book.BookLocation,
		&book.BookPrice, &book.NewPages,
		&enableProposed, &enableDynamicUpdate, &book.LastOpenedFromListID,
		&pagesJSON,
	)
	if err != nil {
		return nil, err
	}

	// Convert integers to booleans
	book.Checked = checked != 0
	book.FileIsMissing = fileIsMissing != 0
	book.ComicInfoIsDirty = comicInfoIsDirty != 0
	book.EnableProposed = enableProposed != 0
	book.EnableDynamicUpdate = enableDynamicUpdate != 0

	// Parse times
	if openedTime.Valid {
		book.OpenedTime = parseComicTime(openedTime.String)
	}
	if addedTime.Valid {
		book.AddedTime = parseComicTime(addedTime.String)
	}
	if releasedTime.Valid {
		book.ReleasedTime = parseComicTime(releasedTime.String)
	}
	if fileModifiedTime.Valid {
		book.FileModifiedTime = parseComicTime(fileModifiedTime.String)
	}
	if fileCreationTime.Valid {
		book.FileCreationTime = parseComicTime(fileCreationTime.String)
	}

	// Parse pages JSON
	if pagesJSON != "" && pagesJSON != "null" {
		if err := json.Unmarshal([]byte(pagesJSON), &book.Pages); err != nil {
			return nil, fmt.Errorf("unmarshal pages: %w", err)
		}
	}

	return &book, nil
}

func scanBookFromRows(rows *sql.Rows) (*library.ComicBook, error) {
	return scanBook(rows)
}

func scanList(s scanner) (*library.ComicListItem, error) {
	var list library.ComicListItem
	var description sql.NullString
	var matchersJSON string

	err := s.Scan(
		&list.ID, &list.Name, &list.Type, &description,
		&list.Favorite, &list.Collapsed, &list.MatcherMode, &matchersJSON, &list.BookCount,
	)
	if err != nil {
		return nil, err
	}

	if description.Valid {
		list.Description = description.String
	}

	// Parse matchers
	if matchersJSON != "" && matchersJSON != "null" {
		if err := json.Unmarshal([]byte(matchersJSON), &list.Matchers); err != nil {
			return nil, fmt.Errorf("unmarshal matchers: %w", err)
		}
	}

	return &list, nil
}

func (db *DB) loadBookCustomValues(book *library.ComicBook) error {
	rows, err := db.Query("SELECT key, value FROM book_custom_values WHERE book_id = ?", book.ID)
	if err != nil {
		return fmt.Errorf("query custom values: %w", err)
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return fmt.Errorf("scan custom value: %w", err)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate custom values: %w", err)
	}

	if len(parts) > 0 {
		book.CustomValuesStore = "," + joinStrings(parts, ",")
	}

	return nil
}

func (db *DB) loadBookTags(book *library.ComicBook) error {
	rows, err := db.Query("SELECT tag FROM book_tags WHERE book_id = ?", book.ID)
	if err != nil {
		return fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tags: %w", err)
	}

	if len(tags) > 0 {
		book.Tags = joinStrings(tags, ", ")
	}

	return nil
}

func (db *DB) loadReadingListItems(list *library.ComicListItem) error {
	rows, err := db.Query(`
		SELECT book_id FROM reading_list_items
		WHERE list_id = ?
		ORDER BY position
	`, list.ID)
	if err != nil {
		return fmt.Errorf("query reading list items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var bookID string
		if err := rows.Scan(&bookID); err != nil {
			return fmt.Errorf("scan reading list item: %w", err)
		}
		list.Items = append(list.Items, library.ComicReadingListItem{ID: bookID})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate reading list items: %w", err)
	}

	return nil
}

// InsertList creates a new list record in the database.
func (db *DB) InsertList(list *library.ComicListItem) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := db.insertList(tx, list, "", ""); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateListRecord updates an existing list record in the database.
func (db *DB) UpdateListRecord(list *library.ComicListItem) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := db.updateList(tx, list, "", ""); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteList removes a list (and cascades to children and reading_list_items).
func (db *DB) DeleteList(id string) error {
	_, err := db.Exec("DELETE FROM lists WHERE id = ?", id)
	return err
}

// MoveList updates the parent_id of a list or folder. parentID="" sets parent to NULL (root).
func (db *DB) MoveList(id, parentID string) error {
	if parentID == "" {
		_, err := db.Exec("UPDATE lists SET parent_id = NULL WHERE id = ?", id)
		return err
	}
	_, err := db.Exec("UPDATE lists SET parent_id = ? WHERE id = ?", parentID, id)
	return err
}

func parseComicTime(s string) library.ComicTime {
	var ct library.ComicTime
	ct.UnmarshalText([]byte(s))
	return ct
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// UpdateBookFields persists every mutable field of book - used by
// everything that writes to a live book outside of an XML import: reverse
// sync (reading progress, rating, notes, review, summary, checked, tags),
// scan-info (ScanInformation), and CBZ-convert (FilePath, PageCount).
//
// Originally covered only the reverse-sync subset; comic-server-4vq found
// that gap by way of scan-info and CBZ-convert silently no-op'ing on the
// SQLite backend - ScanInformation/FilePath/PageCount (and everything
// else) weren't in the UPDATE statement at all, so a caller-side change to
// those fields never reached the database. Now mirrors the full column
// list insertBook/updateBook (import.go) write, so any field a future
// feature sets on a *library.ComicBook and passes to
// Backend.UpdateBook(s) actually persists - there's no longer a second,
// narrower list to remember to extend.
//
// Deliberately does NOT touch import_hash: that column is compared only
// against a hash computed from freshly-parsed XML during Import()/Reload,
// never against live DB content, so a live-write path touching it would
// be meaningless (see comic-server-aio for the reimport-merge work that
// actually depends on import_hash's semantics staying exactly that).
func (db *DB) UpdateBookFields(book *library.ComicBook) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	pagesJSON, err := json.Marshal(book.Pages)
	if err != nil {
		return fmt.Errorf("marshal pages: %w", err)
	}

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
			file_size = ?, file_modified_time = ?, file_creation_time = ?,
			isbn = ?, book_age = ?, book_condition = ?, book_store = ?, book_owner = ?,
			book_collection_status = ?, book_notes = ?, book_location = ?,
			book_price = ?, new_pages = ?,
			enable_proposed = ?, enable_dynamic_update = ?, last_opened_from_list_id = ?,
			pages = ?, updated_at = datetime('now')
		WHERE id = ?
	`,
		book.FilePath, book.Title, book.Series, book.Number, book.Volume, book.Year, book.Month, book.Day,
		book.Publisher, book.Imprint, book.Genre, book.Format, book.AgeRating, book.LanguageISO,
		book.Summary, book.Notes, book.Review, book.StoryArc, book.SeriesGroup,
		book.AlternateSeries, book.AlternateNumber, book.AlternateCount, book.Count,
		book.Writer, book.Penciller, book.Inker, book.Colorist, book.Letterer, book.CoverArtist, book.Editor, book.Translator,
		book.Characters, book.Teams, book.Locations, book.MainCharacterOrTeam,
		book.CurrentPage, book.LastPage, book.LastPageRead, book.OpenCount, formatComicTime(book.OpenedTime),
		book.Rating, book.CommunityRating,
		boolToInt(book.Checked), boolToInt(book.FileIsMissing), boolToInt(book.ComicInfoIsDirty),
		book.PageCount, book.Web, book.ScanInformation, book.SeriesComplete,
		book.BlackAndWhite, book.Manga,
		book.PreferredFrontCover, formatComicTime(book.AddedTime), formatComicTime(book.ReleasedTime),
		book.FileSize, formatComicTime(book.FileModifiedTime), formatComicTime(book.FileCreationTime),
		book.ISBN, book.BookAge, book.BookCondition, book.BookStore, book.BookOwner,
		book.BookCollectionStatus, book.BookNotes, book.BookLocation,
		book.BookPrice, book.NewPages,
		boolToInt(book.EnableProposed), boolToInt(book.EnableDynamicUpdate), book.LastOpenedFromListID,
		string(pagesJSON),
		book.ID,
	)
	if err != nil {
		return fmt.Errorf("update book %s: %w", book.ID, err)
	}

	// Replace tags unconditionally - book.Tags == "" must clear them, not
	// leave stale rows behind (see comic-server-dfs).
	if _, err := tx.Exec("DELETE FROM book_tags WHERE book_id = ?", book.ID); err != nil {
		return fmt.Errorf("delete tags: %w", err)
	}
	tags := splitTags(book.Tags)
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if _, err := tx.Exec("INSERT INTO book_tags (book_id, tag) VALUES (?, ?)", book.ID, tag); err != nil {
			return fmt.Errorf("insert tag: %w", err)
		}
	}

	// Replace custom values unconditionally, same reasoning as tags -
	// matches insertBook/updateBook's own delete+reinsert pattern
	// (import.go) rather than leaving stale keys behind.
	if _, err := tx.Exec("DELETE FROM book_custom_values WHERE book_id = ?", book.ID); err != nil {
		return fmt.Errorf("delete custom values: %w", err)
	}
	if err := db.insertBookCustomValues(tx, book); err != nil {
		return fmt.Errorf("insert custom values: %w", err)
	}

	return tx.Commit()
}

func formatComicTime(t library.ComicTime) string {
	if t.Time.IsZero() {
		return ""
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05Z")
}

func splitTags(tags string) []string {
	var result []string
	for _, tag := range splitString(tags, ",") {
		tag = trimSpace(tag)
		if tag != "" {
			result = append(result, tag)
		}
	}
	return result
}

func splitString(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
