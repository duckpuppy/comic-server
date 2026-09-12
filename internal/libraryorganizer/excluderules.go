package libraryorganizer

import (
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ExcludeRule is one condition in a profile's exclude-rule set - mirrors
// lobookmover's ExcludeRule (locommon.py). Field is the human-readable
// name as stored in losettingsx.dat ("Tags", "File Path", ...), not a Go
// field name - see excludeRuleFieldTable.
type ExcludeRule struct {
	Field    string
	Operator string
	Value    string
}

// ExcludeConfig is the subset of a profile's exclude settings
// ShouldMove needs: the flat rule list plus ExcludeMode/ExcludeOperator.
// Ground-truthed against locommon.py's check_metadata_rules - NOT
// guessed: this is a flat rule list (no nested ExcludeGroup support at
// the profile level, despite the ExcludeGroup class existing in the
// Python source for it), and the semantics are exact, not approximated:
//   - zero rules configured -> ALWAYS move the book, regardless of
//     ExcludeMode (an empty rule set can't exclude anything).
//   - ExcludeOperator "Any" needs at least one rule to match; "All" needs
//     every rule to match. This produces "qualifies".
//   - ExcludeMode "Only" -> move the book only if it qualifies.
//   - ExcludeMode "Do not" -> move the book only if it does NOT qualify
//     (qualifying means "excluded").
type ExcludeConfig struct {
	Rules           []ExcludeRule
	ExcludeMode     string // "Only" or "Do not"
	ExcludeOperator string // "Any" or "All"
}

// ShouldMove reports whether book passes profile's exclude-rule
// filtering (see ExcludeConfig's doc comment for the exact semantics).
// ok is false if any rule references a field or operator this build
// doesn't support - callers (comic-server-3bz.4's preview) should treat
// that as a flagged failure, not silently include or exclude the book.
func ShouldMove(book *library.ComicBook, cfg ExcludeConfig) (move bool, ok bool) {
	if len(cfg.Rules) == 0 {
		return true, true
	}

	count := 0
	total := 0
	for _, rule := range cfg.Rules {
		matched, valid := evaluateExcludeRule(book, rule)
		if !valid {
			return false, false
		}
		total++
		if matched {
			count++
		}
	}

	qualifies := false
	if cfg.ExcludeOperator == "All" {
		qualifies = count == total
	} else { // "Any" is the real default; treat anything else as "Any" too, matching the Python's own implicit else-branch
		qualifies = count > 0
	}

	if cfg.ExcludeMode == "Do not" {
		return !qualifies, true
	}
	// "Only" is the real default; matches the Python's own explicit
	// "Only" branch plus its implicit fallthrough for any other value.
	return qualifies, true
}

func evaluateExcludeRule(book *library.ComicBook, rule ExcludeRule) (matched bool, ok bool) {
	fieldValue, valid := excludeRuleFieldValue(book, rule.Field)
	if !valid {
		return false, false
	}
	return compareExcludeRule(fieldValue, rule.Operator, rule.Value), true
}

// compareExcludeRule mirrors ExcludeRule.calculate_book_should_be_moved
// exactly: case-sensitive string comparison (the Python uses plain `==`/
// `in`, no case-folding, unlike internal/library's smart-list matchers
// which default to IgnoreCase - this is a real, deliberate difference
// from that engine, not an oversight, so LO exclude rules get their own
// small evaluator rather than reusing smartlist.go's).
func compareExcludeRule(fieldValue, operator, value string) bool {
	switch operator {
	case "is":
		return fieldValue == value
	case "is not":
		return fieldValue != value
	case "contains":
		return strings.Contains(fieldValue, value)
	case "does not contain":
		return !strings.Contains(fieldValue, value)
	case "greater than":
		if vi, err1 := strconv.Atoi(value); err1 == nil {
			if fi, err2 := strconv.Atoi(fieldValue); err2 == nil {
				return fi > vi
			}
		}
		return fieldValue > value
	case "less than":
		if vi, err1 := strconv.Atoi(value); err1 == nil {
			if fi, err2 := strconv.Atoi(fieldValue); err2 == nil {
				return fi < vi
			}
		}
		return fieldValue < value
	default:
		return false
	}
}

// ExcludeRuleFields lists every field name excludeRuleFieldValue actually
// supports, in the same order as its switch statement - exported for the
// rule editor UI's field picker (comic-server-7ecr), so that list has one
// source of truth instead of being hand-copied into the API layer and
// risking drift from what evaluateExcludeRule actually accepts.
var ExcludeRuleFields = []string{
	"Tags", "File Path", "File Name", "File Format", "Series", "Title", "Format",
	"Volume", "Year", "Number", "Count", "Month", "Day", "Publisher", "Imprint",
	"Genre", "Web", "Age Rating", "Language", "Writer", "Penciller", "Inker",
	"Colorist", "Letterer", "Cover Artist", "Editor", "Characters", "Teams",
	"Locations", "Main Character Or Team", "Story Arc", "Series Group", "Notes",
	"Review", "Scan Information", "Alternate Series", "Alternate Number",
	"Alternate Count", "Black And White", "Manga", "Series Complete", "Rating",
}

// ExcludeRuleOperators lists every operator compareExcludeRule accepts.
var ExcludeRuleOperators = []string{"is", "is not", "contains", "does not contain", "greater than", "less than"}

// excludeRuleFieldValue looks up rule.Field (a losettingsx.dat human
// field name, e.g. "File Path") against book, returning its text value.
// Scoped to fields with a direct ComicBook property - StartYear/
// StartMonth/EndYear/EndMonth (need an "earliest/last book in this
// series" lookup), Counter (persistent cross-run state), ReadPercentage
// and First Letter (need bucketing/derivation logic) are real
// name_to_field entries but not ported, matching PathMaker's own
// documented field-support gap (comic-server-3bz.1) for the same reason:
// none of the user's real profiles' exclude rules use them (the real
// "Archive" profile only uses Tags and File Path).
func excludeRuleFieldValue(book *library.ComicBook, field string) (string, bool) {
	switch field {
	case "Tags":
		return book.Tags, true
	case "File Path":
		return book.FilePath, true
	case "File Name":
		return baseNameNoExt(book.FilePath), true
	case "File Format":
		return extOf(book.FilePath), true
	case "Series":
		return shadowSeries(book), true
	case "Title":
		return shadowTitle(book), true
	case "Format":
		return shadowFormat(book), true
	case "Volume":
		return intFieldText(shadowVolume(book)), true
	case "Year":
		return intFieldText(shadowYear(book)), true
	case "Number":
		return shadowNumber(book), true
	case "Count":
		return intFieldText(shadowCount(book)), true
	case "Month":
		return intFieldText(book.Month), true
	case "Day":
		return intFieldText(book.Day), true
	case "Publisher":
		return book.Publisher, true
	case "Imprint":
		return book.Imprint, true
	case "Genre":
		return book.Genre, true
	case "Web":
		return book.Web, true
	case "Age Rating":
		return book.AgeRating, true
	case "Language":
		return book.LanguageISO, true
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
	case "Cover Artist":
		return book.CoverArtist, true
	case "Editor":
		return book.Editor, true
	case "Characters":
		return book.Characters, true
	case "Teams":
		return book.Teams, true
	case "Locations":
		return book.Locations, true
	case "Main Character Or Team":
		return book.MainCharacterOrTeam, true
	case "Story Arc":
		return book.StoryArc, true
	case "Series Group":
		return book.SeriesGroup, true
	case "Notes":
		return book.Notes, true
	case "Review":
		return book.Review, true
	case "Scan Information":
		return book.ScanInformation, true
	case "Alternate Series":
		return book.AlternateSeries, true
	case "Alternate Number":
		return book.AlternateNumber, true
	case "Alternate Count":
		return intFieldText(book.AlternateCount), true
	case "Black And White":
		return book.BlackAndWhite, true
	case "Manga":
		return book.Manga, true
	case "Series Complete":
		return book.SeriesComplete, true
	case "Rating":
		return strconv.FormatFloat(book.Rating, 'f', -1, 64), true
	default:
		return "", false
	}
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
