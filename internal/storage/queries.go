package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/duckpuppy/comic-server/internal/library"
)

// GetBook retrieves a single book by ID.
func (db *DB) GetBook(id string) (*library.ComicBook, error) {
	row := db.QueryRow(`
		SELECT
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
			pages
		FROM books WHERE id = ?
	`, id)

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
	rows, err := db.Query(`
		SELECT
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
			pages
		FROM books
	`)
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

	// Load custom values and tags for all books
	for i := range books {
		if err := db.loadBookCustomValues(&books[i]); err != nil {
			return nil, err
		}
		if err := db.loadBookTags(&books[i]); err != nil {
			return nil, err
		}
	}

	return books, nil
}

// GetBookCount returns the total number of books in the database.
func (db *DB) GetBookCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM books").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count books: %w", err)
	}
	return count, nil
}

// GetList retrieves a single list by ID.
func (db *DB) GetList(id string) (*library.ComicListItem, error) {
	row := db.QueryRow(`
		SELECT id, name, type, description, favorite, collapsed, matcher_mode, matchers, book_count
		FROM lists WHERE id = ?
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
