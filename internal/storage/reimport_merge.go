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

// diffBookColumns compares old (the book exactly as it was parsed from XML
// at the last import - see xml_snapshot), live (the book's CURRENT row in
// the database, which may hold a comic-server-side edit made since that
// import), and new (freshly parsed from the current XML), and returns only
// the columns that should actually change on this merge. Comparing stored
// representations (formatTime output, boolToInt, marshaled JSON) rather
// than the Go struct fields directly sidesteps ComicTime/float equality
// edge cases and exactly matches "would this column's value on disk
// actually be different" - which is the only thing that matters for
// deciding whether to touch it.
//
// This is comic-server-aio's core merge, plus comic-server-cfi's flip of
// its conflict rule: a field only gets the new XML value when the XML
// actually changed it (new != old) AND comic-server hasn't independently
// changed it since the last import (live == old). If comic-server's live
// value has already diverged from old, that's a genuine conflict - a
// device reverse-synced a new CurrentPage, a scan-info run set
// ScanInformation, etc. - and comic-server's live value wins by being left
// alone, not overwritten by the (now comparatively stale) XML. This was
// the reverse under aio's original "XML wins" rule; that stopped being
// the right default once ComicRack Desktop stopped being a live co-author
// of the library and its XML became a periodic, essentially-frozen import
// source instead (see comic-server-cfi).
func diffBookColumns(old, live, new *library.ComicBook) []fieldChange {
	var changes []fieldChange

	str := func(column, oldV, liveV, newV string) {
		if oldV != newV && liveV == oldV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	i := func(column string, oldV, liveV, newV int) {
		if oldV != newV && liveV == oldV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	i64 := func(column string, oldV, liveV, newV int64) {
		if oldV != newV && liveV == oldV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	f := func(column string, oldV, liveV, newV float64) {
		if oldV != newV && liveV == oldV {
			changes = append(changes, fieldChange{column, newV})
		}
	}
	b := func(column string, oldV, liveV, newV bool) {
		if oldV != newV && liveV == oldV {
			changes = append(changes, fieldChange{column, boolToInt(newV)})
		}
	}
	tm := func(column string, oldV, liveV, newV library.ComicTime) {
		oldS, liveS, newS := formatTime(oldV), formatTime(liveV), formatTime(newV)
		if oldS != newS && liveS == oldS {
			changes = append(changes, fieldChange{column, newS})
		}
	}

	str("file_path", old.FilePath, live.FilePath, new.FilePath)
	str("title", old.Title, live.Title, new.Title)
	str("series", old.Series, live.Series, new.Series)
	str("number", old.Number, live.Number, new.Number)
	i("volume", old.Volume, live.Volume, new.Volume)
	i("year", old.Year, live.Year, new.Year)
	i("month", old.Month, live.Month, new.Month)
	i("day", old.Day, live.Day, new.Day)
	str("publisher", old.Publisher, live.Publisher, new.Publisher)
	str("imprint", old.Imprint, live.Imprint, new.Imprint)
	str("genre", old.Genre, live.Genre, new.Genre)
	str("format", old.Format, live.Format, new.Format)
	str("age_rating", old.AgeRating, live.AgeRating, new.AgeRating)
	str("language_iso", old.LanguageISO, live.LanguageISO, new.LanguageISO)
	str("summary", old.Summary, live.Summary, new.Summary)
	str("notes", old.Notes, live.Notes, new.Notes)
	str("review", old.Review, live.Review, new.Review)
	str("story_arc", old.StoryArc, live.StoryArc, new.StoryArc)
	str("series_group", old.SeriesGroup, live.SeriesGroup, new.SeriesGroup)
	str("alternate_series", old.AlternateSeries, live.AlternateSeries, new.AlternateSeries)
	str("alternate_number", old.AlternateNumber, live.AlternateNumber, new.AlternateNumber)
	i("alternate_count", old.AlternateCount, live.AlternateCount, new.AlternateCount)
	i("count", old.Count, live.Count, new.Count)
	str("writer", old.Writer, live.Writer, new.Writer)
	str("penciller", old.Penciller, live.Penciller, new.Penciller)
	str("inker", old.Inker, live.Inker, new.Inker)
	str("colorist", old.Colorist, live.Colorist, new.Colorist)
	str("letterer", old.Letterer, live.Letterer, new.Letterer)
	str("cover_artist", old.CoverArtist, live.CoverArtist, new.CoverArtist)
	str("editor", old.Editor, live.Editor, new.Editor)
	str("translator", old.Translator, live.Translator, new.Translator)
	str("characters", old.Characters, live.Characters, new.Characters)
	str("teams", old.Teams, live.Teams, new.Teams)
	str("locations", old.Locations, live.Locations, new.Locations)
	str("main_character_or_team", old.MainCharacterOrTeam, live.MainCharacterOrTeam, new.MainCharacterOrTeam)
	i("current_page", old.CurrentPage, live.CurrentPage, new.CurrentPage)
	i("last_page", old.LastPage, live.LastPage, new.LastPage)
	i("last_page_read", old.LastPageRead, live.LastPageRead, new.LastPageRead)
	i("open_count", old.OpenCount, live.OpenCount, new.OpenCount)
	tm("opened_time", old.OpenedTime, live.OpenedTime, new.OpenedTime)
	f("rating", old.Rating, live.Rating, new.Rating)
	f("community_rating", old.CommunityRating, live.CommunityRating, new.CommunityRating)
	b("checked", old.Checked, live.Checked, new.Checked)
	b("file_is_missing", old.FileIsMissing, live.FileIsMissing, new.FileIsMissing)
	b("comic_info_is_dirty", old.ComicInfoIsDirty, live.ComicInfoIsDirty, new.ComicInfoIsDirty)
	i("page_count", old.PageCount, live.PageCount, new.PageCount)
	str("web", old.Web, live.Web, new.Web)
	str("scan_information", old.ScanInformation, live.ScanInformation, new.ScanInformation)
	str("series_complete", old.SeriesComplete, live.SeriesComplete, new.SeriesComplete)
	str("black_and_white", old.BlackAndWhite, live.BlackAndWhite, new.BlackAndWhite)
	str("manga", old.Manga, live.Manga, new.Manga)
	i("preferred_front_cover", old.PreferredFrontCover, live.PreferredFrontCover, new.PreferredFrontCover)
	tm("added_time", old.AddedTime, live.AddedTime, new.AddedTime)
	tm("released_time", old.ReleasedTime, live.ReleasedTime, new.ReleasedTime)
	i64("file_size", old.FileSize, live.FileSize, new.FileSize)
	tm("file_modified_time", old.FileModifiedTime, live.FileModifiedTime, new.FileModifiedTime)
	tm("file_creation_time", old.FileCreationTime, live.FileCreationTime, new.FileCreationTime)
	str("isbn", old.ISBN, live.ISBN, new.ISBN)
	str("book_age", old.BookAge, live.BookAge, new.BookAge)
	str("book_condition", old.BookCondition, live.BookCondition, new.BookCondition)
	str("book_store", old.BookStore, live.BookStore, new.BookStore)
	str("book_owner", old.BookOwner, live.BookOwner, new.BookOwner)
	str("book_collection_status", old.BookCollectionStatus, live.BookCollectionStatus, new.BookCollectionStatus)
	str("book_notes", old.BookNotes, live.BookNotes, new.BookNotes)
	str("book_location", old.BookLocation, live.BookLocation, new.BookLocation)
	f("book_price", old.BookPrice, live.BookPrice, new.BookPrice)
	i("new_pages", old.NewPages, live.NewPages, new.NewPages)
	b("enable_proposed", old.EnableProposed, live.EnableProposed, new.EnableProposed)
	b("enable_dynamic_update", old.EnableDynamicUpdate, live.EnableDynamicUpdate, new.EnableDynamicUpdate)
	str("last_opened_from_list_id", old.LastOpenedFromListID, live.LastOpenedFromListID, new.LastOpenedFromListID)

	oldPagesJSON, _ := json.Marshal(old.Pages)
	livePagesJSON, _ := json.Marshal(live.Pages)
	newPagesJSON, _ := json.Marshal(new.Pages)
	if string(oldPagesJSON) != string(newPagesJSON) && string(livePagesJSON) == string(oldPagesJSON) {
		changes = append(changes, fieldChange{"pages", string(newPagesJSON)})
	}

	return changes
}

// liveBookSnapshot fetches book id's CURRENT row (columns, tags, and
// custom values) within tx - the comic-server-side state to protect from
// being overwritten by a stale XML value during a merge (see
// diffBookColumns and comic-server-cfi). Reads through tx rather than
// db's own connection pool so this always reflects exactly what this
// transaction would otherwise be about to overwrite, not a separately-
// snapshotted view from another connection.
func (db *DB) liveBookSnapshot(tx *sql.Tx, id string) (*library.ComicBook, error) {
	row := tx.QueryRow("SELECT "+booksSelectColumns+" FROM books WHERE id = ?", id)
	book, err := scanBook(row)
	if err != nil {
		return nil, fmt.Errorf("get live book %s: %w", id, err)
	}

	tagRows, err := tx.Query("SELECT tag FROM book_tags WHERE book_id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("query live tags for %s: %w", id, err)
	}
	var tags []string
	for tagRows.Next() {
		var tag string
		if err := tagRows.Scan(&tag); err != nil {
			tagRows.Close()
			return nil, fmt.Errorf("scan live tag for %s: %w", id, err)
		}
		tags = append(tags, tag)
	}
	tagRows.Close()
	if err := tagRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate live tags for %s: %w", id, err)
	}
	if len(tags) > 0 {
		book.Tags = joinStrings(tags, ", ")
	}

	cvRows, err := tx.Query("SELECT key, value FROM book_custom_values WHERE book_id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("query live custom values for %s: %w", id, err)
	}
	var parts []string
	for cvRows.Next() {
		var key, value string
		if err := cvRows.Scan(&key, &value); err != nil {
			cvRows.Close()
			return nil, fmt.Errorf("scan live custom value for %s: %w", id, err)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	cvRows.Close()
	if err := cvRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate live custom values for %s: %w", id, err)
	}
	if len(parts) > 0 {
		book.CustomValuesStore = "," + joinStrings(parts, ",")
	}

	return book, nil
}

// mergeUpdateBook applies a field-level merge (comic-server-aio) instead
// of updateBook's whole-row overwrite: only columns that genuinely
// changed in the XML AND haven't been independently edited live since the
// last import are touched (comic-server-cfi), plus tags/custom values
// under the same rule. import_hash, updated_at, deleted_at, and
// xml_snapshot always advance regardless of which (if any) content
// columns changed - their job is tracking "what does the XML currently
// say", independent of how much of that made it into the live row.
//
// hasSnapshot is false only for a row migrated from pre-v4 with no prior
// xml_snapshot to diff against (see migrateV3ToV4 and importBooks) - in
// that case oldSnapshot is a zero-valued stand-in, not a real "what the
// XML last said," so there's no genuine baseline to detect a live
// divergence against. Rather than fetch the live row and compare it
// against that meaningless zero value (which would wrongly treat every
// already-populated field as "in conflict" and withhold it), this
// degrades to the pre-cfi behavior for that one row: apply every field
// that differs from the zero value, i.e. a full overwrite - same as
// before this merge logic existed at all.
func (db *DB) mergeUpdateBook(tx *sql.Tx, oldSnapshot, newBook *library.ComicBook, hash string, hasSnapshot bool) error {
	live := oldSnapshot
	if hasSnapshot {
		var err error
		live, err = db.liveBookSnapshot(tx, newBook.ID)
		if err != nil {
			return err
		}
	}

	changes := diffBookColumns(oldSnapshot, live, newBook)

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

	if oldSnapshot.Tags != newBook.Tags && live.Tags == oldSnapshot.Tags {
		if _, err := tx.Exec("DELETE FROM book_tags WHERE book_id = ?", newBook.ID); err != nil {
			return fmt.Errorf("delete tags: %w", err)
		}
		if err := db.insertBookTags(tx, newBook); err != nil {
			return fmt.Errorf("insert tags: %w", err)
		}
	}

	if oldSnapshot.CustomValuesStore != newBook.CustomValuesStore && live.CustomValuesStore == oldSnapshot.CustomValuesStore {
		if _, err := tx.Exec("DELETE FROM book_custom_values WHERE book_id = ?", newBook.ID); err != nil {
			return fmt.Errorf("delete custom values: %w", err)
		}
		if err := db.insertBookCustomValues(tx, newBook); err != nil {
			return fmt.Errorf("insert custom values: %w", err)
		}
	}

	return nil
}
