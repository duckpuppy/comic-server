package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/duckpuppy/comic-server/internal/library"
)

// fieldChange is one books-table column whose stored value needs to
// change, produced by diffBookColumns.
type fieldChange struct {
	column string
	value  any
}

// diffBookColumns compares oldSnapshot (the book exactly as it was parsed
// from XML at the last import - see xml_snapshot) against newBook (freshly
// parsed from the current XML) and returns only the columns whose STORED
// representation actually changed. Comparing stored representations
// (formatTime output, boolToInt, marshaled JSON) rather than the Go struct
// fields directly sidesteps ComicTime/float equality edge cases and
// exactly matches "would this column's value on disk actually be
// different" - which is the only thing that matters for deciding whether
// to touch it.
//
// This is the core of comic-server-aio: a reimport applies ONLY the
// fields that genuinely changed in the XML since the last import, leaving
// every other column (which may hold a live comic-server-side edit -
// ScanInformation from scan-info, FilePath/PageCount from CBZ-convert,
// reading progress from reverse sync) untouched. Deliberately does not
// cover tags or custom values - those live in separate join tables and
// are diffed/reapplied by the caller (mergeUpdateBook).
func diffBookColumns(old, new *library.ComicBook) []fieldChange {
	var changes []fieldChange

	str := func(column, oldV, newV string) {
		if oldV != newV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	i := func(column string, oldV, newV int) {
		if oldV != newV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	i64 := func(column string, oldV, newV int64) {
		if oldV != newV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	f := func(column string, oldV, newV float64) {
		if oldV != newV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	b := func(column string, oldV, newV bool) {
		if oldV != newV {
			changes = append(changes, fieldChange{column, boolToInt(newV)})
		}
	}
	tm := func(column string, oldV, newV library.ComicTime) {
		oldS, newS := formatTime(oldV), formatTime(newV)
		if oldS != newS {
			changes = append(changes, fieldChange{column, newS})
		}
	}

	str("file_path", old.FilePath, new.FilePath)
	str("title", old.Title, new.Title)
	str("series", old.Series, new.Series)
	str("number", old.Number, new.Number)
	i("volume", old.Volume, new.Volume)
	i("year", old.Year, new.Year)
	i("month", old.Month, new.Month)
	i("day", old.Day, new.Day)
	str("publisher", old.Publisher, new.Publisher)
	str("imprint", old.Imprint, new.Imprint)
	str("genre", old.Genre, new.Genre)
	str("format", old.Format, new.Format)
	str("age_rating", old.AgeRating, new.AgeRating)
	str("language_iso", old.LanguageISO, new.LanguageISO)
	str("summary", old.Summary, new.Summary)
	str("notes", old.Notes, new.Notes)
	str("review", old.Review, new.Review)
	str("story_arc", old.StoryArc, new.StoryArc)
	str("series_group", old.SeriesGroup, new.SeriesGroup)
	str("alternate_series", old.AlternateSeries, new.AlternateSeries)
	str("alternate_number", old.AlternateNumber, new.AlternateNumber)
	i("alternate_count", old.AlternateCount, new.AlternateCount)
	i("count", old.Count, new.Count)
	str("writer", old.Writer, new.Writer)
	str("penciller", old.Penciller, new.Penciller)
	str("inker", old.Inker, new.Inker)
	str("colorist", old.Colorist, new.Colorist)
	str("letterer", old.Letterer, new.Letterer)
	str("cover_artist", old.CoverArtist, new.CoverArtist)
	str("editor", old.Editor, new.Editor)
	str("translator", old.Translator, new.Translator)
	str("characters", old.Characters, new.Characters)
	str("teams", old.Teams, new.Teams)
	str("locations", old.Locations, new.Locations)
	str("main_character_or_team", old.MainCharacterOrTeam, new.MainCharacterOrTeam)
	i("current_page", old.CurrentPage, new.CurrentPage)
	i("last_page", old.LastPage, new.LastPage)
	i("last_page_read", old.LastPageRead, new.LastPageRead)
	i("open_count", old.OpenCount, new.OpenCount)
	tm("opened_time", old.OpenedTime, new.OpenedTime)
	f("rating", old.Rating, new.Rating)
	f("community_rating", old.CommunityRating, new.CommunityRating)
	b("checked", old.Checked, new.Checked)
	b("file_is_missing", old.FileIsMissing, new.FileIsMissing)
	b("comic_info_is_dirty", old.ComicInfoIsDirty, new.ComicInfoIsDirty)
	i("page_count", old.PageCount, new.PageCount)
	str("web", old.Web, new.Web)
	str("scan_information", old.ScanInformation, new.ScanInformation)
	str("series_complete", old.SeriesComplete, new.SeriesComplete)
	str("black_and_white", old.BlackAndWhite, new.BlackAndWhite)
	str("manga", old.Manga, new.Manga)
	i("preferred_front_cover", old.PreferredFrontCover, new.PreferredFrontCover)
	tm("added_time", old.AddedTime, new.AddedTime)
	tm("released_time", old.ReleasedTime, new.ReleasedTime)
	i64("file_size", old.FileSize, new.FileSize)
	tm("file_modified_time", old.FileModifiedTime, new.FileModifiedTime)
	tm("file_creation_time", old.FileCreationTime, new.FileCreationTime)
	str("isbn", old.ISBN, new.ISBN)
	str("book_age", old.BookAge, new.BookAge)
	str("book_condition", old.BookCondition, new.BookCondition)
	str("book_store", old.BookStore, new.BookStore)
	str("book_owner", old.BookOwner, new.BookOwner)
	str("book_collection_status", old.BookCollectionStatus, new.BookCollectionStatus)
	str("book_notes", old.BookNotes, new.BookNotes)
	str("book_location", old.BookLocation, new.BookLocation)
	f("book_price", old.BookPrice, new.BookPrice)
	i("new_pages", old.NewPages, new.NewPages)
	b("enable_proposed", old.EnableProposed, new.EnableProposed)
	b("enable_dynamic_update", old.EnableDynamicUpdate, new.EnableDynamicUpdate)
	str("last_opened_from_list_id", old.LastOpenedFromListID, new.LastOpenedFromListID)

	oldPagesJSON, _ := json.Marshal(old.Pages)
	newPagesJSON, _ := json.Marshal(new.Pages)
	if string(oldPagesJSON) != string(newPagesJSON) {
		changes = append(changes, fieldChange{"pages", string(newPagesJSON)})
	}

	return changes
}

// mergeUpdateBook applies a field-level merge (comic-server-aio) instead
// of updateBook's whole-row overwrite: only columns that actually
// changed between the last-imported snapshot and the freshly-parsed XML
// are touched, plus tags/custom values if their source strings changed.
// import_hash, updated_at, deleted_at, and xml_snapshot always advance
// regardless of which (if any) content columns changed - their job is
// tracking "what does the XML currently say", independent of how much of
// that made it into the live row.
func (db *DB) mergeUpdateBook(tx *sql.Tx, oldSnapshot, newBook *library.ComicBook, hash string) error {
	changes := diffBookColumns(oldSnapshot, newBook)

	newSnapshotJSON, err := json.Marshal(newBook)
	if err != nil {
		return fmt.Errorf("marshal xml snapshot: %w", err)
	}

	setClauses := "import_hash = ?, updated_at = datetime('now'), deleted_at = NULL, xml_snapshot = ?"
	args := []any{hash, string(newSnapshotJSON)}
	for _, c := range changes {
		setClauses += ", " + c.column + " = ?"
		args = append(args, c.value)
	}
	args = append(args, newBook.ID)

	if _, err := tx.Exec(fmt.Sprintf("UPDATE books SET %s WHERE id = ?", setClauses), args...); err != nil {
		return fmt.Errorf("update book %s: %w", newBook.ID, err)
	}

	if oldSnapshot.Tags != newBook.Tags {
		if _, err := tx.Exec("DELETE FROM book_tags WHERE book_id = ?", newBook.ID); err != nil {
			return fmt.Errorf("delete tags: %w", err)
		}
		if err := db.insertBookTags(tx, newBook); err != nil {
			return fmt.Errorf("insert tags: %w", err)
		}
	}

	if oldSnapshot.CustomValuesStore != newBook.CustomValuesStore {
		if _, err := tx.Exec("DELETE FROM book_custom_values WHERE book_id = ?", newBook.ID); err != nil {
			return fmt.Errorf("delete custom values: %w", err)
		}
		if err := db.insertBookCustomValues(tx, newBook); err != nil {
			return fmt.Errorf("insert custom values: %w", err)
		}
	}

	return nil
}
