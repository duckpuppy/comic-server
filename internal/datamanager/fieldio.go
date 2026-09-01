package datamanager

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// GetFieldString reads field off book as a string, for {FieldName}
// substitution in action values and for the Add/Remove/Replace/
// RegexReplace family of actions, which all need the field's CURRENT
// value to compute a new one. Returns ok=false for an unknown field name
// (custom values are read via library's CustomValuesStore directly, not
// through this built-in-only accessor).
func GetFieldString(book *library.ComicBook, field string) (string, bool) {
	switch field {
	case "Title":
		return book.Title, true
	case "Series":
		return book.Series, true
	case "Number":
		return book.Number, true
	case "Count":
		return strconv.Itoa(book.Count), true
	case "Volume":
		return strconv.Itoa(book.Volume), true
	case "AlternateSeries":
		return book.AlternateSeries, true
	case "AlternateNumber":
		return book.AlternateNumber, true
	case "AlternateCount":
		return strconv.Itoa(book.AlternateCount), true
	case "StoryArc":
		return book.StoryArc, true
	case "SeriesGroup":
		return book.SeriesGroup, true
	case "Summary":
		return book.Summary, true
	case "Notes":
		return book.Notes, true
	case "Review":
		return book.Review, true
	case "Tags":
		return book.Tags, true
	case "Characters":
		return book.Characters, true
	case "Teams":
		return book.Teams, true
	case "MainCharacterOrTeam":
		return book.MainCharacterOrTeam, true
	case "Locations":
		return book.Locations, true
	case "Year":
		return strconv.Itoa(book.Year), true
	case "Month":
		return strconv.Itoa(book.Month), true
	case "Day":
		return strconv.Itoa(book.Day), true
	case "Publisher":
		return book.Publisher, true
	case "Imprint":
		return book.Imprint, true
	case "Format":
		return book.Format, true
	case "LanguageISO":
		return book.LanguageISO, true
	case "Genre":
		return book.Genre, true
	case "Web":
		return book.Web, true
	case "AgeRating":
		return book.AgeRating, true
	case "Writer":
		return book.Writer, true
	case "Penciller":
		return book.Penciller, true
	case "Inker":
		return book.Inker, true
	case "Colorist":
		return book.Colorist, true
	case "Letterer":
		return book.Letterer, true
	case "CoverArtist":
		return book.CoverArtist, true
	case "Editor":
		return book.Editor, true
	case "Translator":
		return book.Translator, true
	case "PageCount":
		return strconv.Itoa(book.PageCount), true
	case "BlackAndWhite":
		return book.BlackAndWhite, true
	case "Manga":
		return book.Manga, true
	case "Rating":
		return strconv.FormatFloat(book.Rating, 'f', -1, 64), true
	case "CommunityRating":
		return strconv.FormatFloat(book.CommunityRating, 'f', -1, 64), true
	case "AddedTime":
		return book.AddedTime.Time.Format(time.RFC3339), true
	case "ReleasedTime":
		return book.ReleasedTime.Time.Format(time.RFC3339), true
	case "Published":
		return time.Date(book.Year, time.Month(max(book.Month, 1)), max(book.Day, 1), 0, 0, 0, 0, time.UTC).Format(time.RFC3339), true
	case "BookPrice":
		return strconv.FormatFloat(book.BookPrice, 'f', -1, 64), true
	case "ISBN":
		return book.ISBN, true
	case "BookAge":
		return book.BookAge, true
	case "BookCondition":
		return book.BookCondition, true
	case "BookStore":
		return book.BookStore, true
	case "BookOwner":
		return book.BookOwner, true
	case "BookCollectionStatus":
		return book.BookCollectionStatus, true
	case "BookNotes":
		return book.BookNotes, true
	case "BookLocation":
		return book.BookLocation, true
	case "Checked":
		return yesNo(book.Checked), true
	case "ComicInfoIsDirty":
		return yesNo(book.ComicInfoIsDirty), true
	case "SeriesComplete":
		return book.SeriesComplete, true
	case "EnableProposed":
		return yesNo(book.EnableProposed), true
	case "ScanInformation":
		return book.ScanInformation, true
	// Read-only fields (dataman.ini's ReadOnlyKeys) - still readable for
	// {FieldName} substitution, just never a SetFieldString target.
	case "FileDirectory":
		return dirOf(book.FilePath), true
	case "FileFormat":
		return strings.ToUpper(strings.TrimPrefix(extOf(book.FilePath), ".")), true
	case "FileIsMissing":
		return yesNo(book.FileIsMissing), true
	case "FileName":
		return baseNameNoExt(book.FilePath), true
	case "FilePath":
		return strings.ReplaceAll(book.FilePath, "\\", "/"), true
	case "HasBeenOpened":
		return yesNo(book.OpenCount > 0), true
	case "HasBeenRead":
		return yesNo(book.OpenCount > 0 && !book.IsUnread()), true
	case "ComicBookIsDirty":
		return "no", true
	default:
		return "", false
	}
}

// SetFieldString writes value to field on book, converting to the field's
// real type as needed. Returns an error for an unknown or read-only field
// (dataman.ini's ReadOnlyKeys) - callers should check builtinFields[field]
// .Writable before calling this for a field name that might be read-only,
// since that's a rule-authoring error, not a runtime data problem.
func SetFieldString(book *library.ComicBook, field, value string) error {
	def, ok := builtinFields[field]
	if !ok {
		return fmt.Errorf("unknown field %q", field)
	}
	if !def.Writable {
		return fmt.Errorf("field %q is read-only", field)
	}

	switch field {
	case "Title":
		book.Title = value
	case "Series":
		book.Series = value
	case "Number":
		book.Number = value
	case "Count":
		return setInt(&book.Count, value)
	case "Volume":
		return setInt(&book.Volume, value)
	case "AlternateSeries":
		book.AlternateSeries = value
	case "AlternateNumber":
		book.AlternateNumber = value
	case "AlternateCount":
		return setInt(&book.AlternateCount, value)
	case "StoryArc":
		book.StoryArc = value
	case "SeriesGroup":
		book.SeriesGroup = value
	case "Summary":
		book.Summary = value
	case "Notes":
		book.Notes = value
	case "Review":
		book.Review = value
	case "Tags":
		book.Tags = value
	case "Characters":
		book.Characters = value
	case "Teams":
		book.Teams = value
	case "MainCharacterOrTeam":
		book.MainCharacterOrTeam = value
	case "Locations":
		book.Locations = value
	case "Year":
		return setInt(&book.Year, value)
	case "Month":
		return setInt(&book.Month, value)
	case "Day":
		return setInt(&book.Day, value)
	case "Publisher":
		book.Publisher = value
	case "Imprint":
		book.Imprint = value
	case "Format":
		book.Format = value
	case "LanguageISO":
		book.LanguageISO = value
	case "Genre":
		book.Genre = value
	case "Web":
		book.Web = value
	case "AgeRating":
		book.AgeRating = value
	case "Writer":
		book.Writer = value
	case "Penciller":
		book.Penciller = value
	case "Inker":
		book.Inker = value
	case "Colorist":
		book.Colorist = value
	case "Letterer":
		book.Letterer = value
	case "CoverArtist":
		book.CoverArtist = value
	case "Editor":
		book.Editor = value
	case "Translator":
		book.Translator = value
	case "PageCount":
		return setInt(&book.PageCount, value)
	case "BlackAndWhite":
		book.BlackAndWhite = value
	case "Manga":
		book.Manga = value
	case "Rating":
		return setFloat(&book.Rating, value)
	case "CommunityRating":
		return setFloat(&book.CommunityRating, value)
	case "AddedTime":
		t, err := dmDateToTime(value)
		if err != nil {
			return err
		}
		book.AddedTime = library.ComicTime{Time: t}
	case "ReleasedTime":
		t, err := dmDateToTime(value)
		if err != nil {
			return err
		}
		book.ReleasedTime = library.ComicTime{Time: t}
	case "Published":
		t, err := dmDateToTime(value)
		if err != nil {
			return err
		}
		book.Year, book.Month, book.Day = t.Year(), int(t.Month()), t.Day()
	case "BookPrice":
		return setFloat(&book.BookPrice, value)
	case "ISBN":
		book.ISBN = value
	case "BookAge":
		book.BookAge = value
	case "BookCondition":
		book.BookCondition = value
	case "BookStore":
		book.BookStore = value
	case "BookOwner":
		book.BookOwner = value
	case "BookCollectionStatus":
		book.BookCollectionStatus = value
	case "BookNotes":
		book.BookNotes = value
	case "BookLocation":
		book.BookLocation = value
	case "Checked":
		book.Checked = isYes(value)
	case "ComicInfoIsDirty":
		book.ComicInfoIsDirty = isYes(value)
	case "SeriesComplete":
		book.SeriesComplete = value
	case "EnableProposed":
		book.EnableProposed = isYes(value)
	case "ScanInformation":
		book.ScanInformation = value
	default:
		return fmt.Errorf("field %q has no writer implemented", field)
	}
	return nil
}

func setInt(dst *int, value string) error {
	if value == "" {
		*dst = 0
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid integer %q: %w", value, err)
	}
	*dst = n
	return nil
}

func setFloat(dst *float64, value string) error {
	if value == "" {
		*dst = 0
		return nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fmt.Errorf("invalid number %q: %w", value, err)
	}
	*dst = f
	return nil
}

func dmDateToTime(value string) (time.Time, error) {
	s, err := dmDateToRFC3339(value)
	if err != nil {
		return time.Time{}, err
	}
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func isYes(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "yes") || strings.EqualFold(strings.TrimSpace(s), "true")
}

func dirOf(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[:idx]
	}
	return ""
}

func extOf(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	base := p
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		base = p[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx > 0 {
		return base[idx:]
	}
	return ""
}

func baseNameNoExt(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	base := p
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		base = p[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx > 0 {
		return base[:idx]
	}
	return base
}
