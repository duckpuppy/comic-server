package storage

import (
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// This file translates a smart list's matcher tree into a SQL WHERE clause
// so SQLiteBackend can narrow the row fetch in the database instead of
// always loading every book and evaluating matchers in Go (see
// comic-server-770).
//
// Safety model: every translated predicate is a SAFE SUPERSET of the true
// in-memory match - it may include some books that don't actually match
// (false positives), but it must never exclude a book that does match. AND
// and OR are both monotonic with respect to supersets (if A ⊇ A' and B ⊇
// B', then A∩B ⊇ A'∩B' and A∪B ⊇ A'∪B'), so combining safe-superset
// predicates with AND/OR - including in nested groups - always yields
// another safe superset. SQLiteBackend.MatchBooks re-runs the exact
// in-memory matcher (library.MatchBooks) on the resulting (small) candidate
// set, so an over-broad SQL predicate only costs a slightly larger row
// fetch - it can never produce a wrong result. If any matcher in the tree
// can't be safely translated, translateMatchers reports ok=false and the
// caller must fall back to loading every book.
//
// This is why string equals/contains/starts/ends all use COLLATE NOCASE
// regardless of the matcher's actual case-sensitivity: a case-insensitive
// match is always a superset of a case-sensitive one, so it's always safe
// - the Go re-check applies the real semantics afterward.

// sqlPredicate is a translated WHERE fragment plus its positional args.
type sqlPredicate struct {
	where string
	args  []any
}

// matcherSQLKind describes how a books-table column should be compared.
type matcherSQLKind int

const (
	kindString    matcherSQLKind = iota // plain text column, case-insensitive compare
	kindNumeric                         // integer or real column, compared as float64 (mirrors Matcher.matchNumeric, which always parses via ParseFloat)
	kindBoolYesNo                       // 0/1 column; only Yes(0)/No(1) operators are safely translatable
	kindEnumYesNo                       // text column storing "Yes"/"No"/"Unknown"/""; Yes(0)/No(1)/Unknown(2) all translatable
)

type matcherSQLColumn struct {
	column string
	kind   matcherSQLKind
}

// matcherSQLColumns maps translatable MatcherTypes to their books-table
// column. Only covers matchers backed directly by a single books-table
// column with straightforward semantics - anything computed (ReadPercentage,
// Directory/File/FullPath, AllProperties, dates, Manga's 4-state logic),
// requiring other tables (Tags, CustomValues), or requiring cross-book
// context (Duplicate, series aggregates, ComicVine enrichment) is
// deliberately left untranslated, falling back to the existing full
// in-memory path.
var matcherSQLColumns = map[library.MatcherType]matcherSQLColumn{
	library.MatcherTypeSeries:               {"series", kindString},
	library.MatcherTypeAlternateSeries:      {"alternate_series", kindString},
	library.MatcherTypeSeriesGroup:          {"series_group", kindString},
	library.MatcherTypeTitle:                {"title", kindString},
	library.MatcherTypePublisher:            {"publisher", kindString},
	library.MatcherTypeImprint:              {"imprint", kindString},
	library.MatcherTypeWriter:               {"writer", kindString},
	library.MatcherTypeGenre:                {"genre", kindString},
	library.MatcherTypeFormat:               {"format", kindString},
	library.MatcherTypeCharacters:           {"characters", kindString},
	library.MatcherTypeTeams:                {"teams", kindString},
	library.MatcherTypeNumber:               {"number", kindString},
	library.MatcherTypeNotes:                {"notes", kindString},
	library.MatcherTypeColorist:             {"colorist", kindString},
	library.MatcherTypeCoverArtist:          {"cover_artist", kindString},
	library.MatcherTypeEditor:               {"editor", kindString},
	library.MatcherTypeInker:                {"inker", kindString},
	library.MatcherTypeLetterer:             {"letterer", kindString},
	library.MatcherTypePenciller:            {"penciller", kindString},
	library.MatcherTypeTranslator:           {"translator", kindString},
	library.MatcherTypeMainCharacterOrTeam:  {"main_character_or_team", kindString},
	library.MatcherTypeLocations:            {"locations", kindString},
	library.MatcherTypeStoryArc:             {"story_arc", kindString},
	library.MatcherTypeAgeRating:            {"age_rating", kindString},
	library.MatcherTypeSummary:              {"summary", kindString},
	library.MatcherTypeReview:               {"review", kindString},
	library.MatcherTypeISBN:                 {"isbn", kindString},
	library.MatcherTypeWeb:                  {"web", kindString},
	library.MatcherTypeBookAge:              {"book_age", kindString},
	library.MatcherTypeBookCondition:        {"book_condition", kindString},
	library.MatcherTypeBookStore:            {"book_store", kindString},
	library.MatcherTypeBookOwner:            {"book_owner", kindString},
	library.MatcherTypeBookCollectionStatus: {"book_collection_status", kindString},
	library.MatcherTypeBookNotes:            {"book_notes", kindString},
	library.MatcherTypeBookLocation:         {"book_location", kindString},
	library.MatcherTypeLanguage:             {"language_iso", kindString},

	library.MatcherTypeYear:            {"year", kindNumeric},
	library.MatcherTypeMonth:           {"month", kindNumeric},
	library.MatcherTypeVolume:          {"volume", kindNumeric},
	library.MatcherTypePageCount:       {"page_count", kindNumeric},
	library.MatcherTypeRating:          {"rating", kindNumeric},
	library.MatcherTypeCommunityRating: {"community_rating", kindNumeric},
	library.MatcherTypeAlternateCount:  {"alternate_count", kindNumeric},
	library.MatcherTypeFileSize:        {"file_size", kindNumeric},

	library.MatcherTypeChecked:   {"checked", kindBoolYesNo},
	library.MatcherTypeIsMissing: {"file_is_missing", kindBoolYesNo},

	library.MatcherTypeSeriesComplete: {"series_complete", kindEnumYesNo},
	library.MatcherTypeBlackAndWhite:  {"black_and_white", kindEnumYesNo},
}

// translateMatchers attempts to build a safe-superset SQL WHERE clause for
// a smart list's matcher tree. mode is "And" or "Or" (list.MatcherMode or a
// nested group's MatcherMode). Returns ok=false if any matcher - at any
// depth - can't be safely translated, in which case the caller must fall
// back to the full in-memory evaluation.
func translateMatchers(mode string, matchers []library.ComicBookMatcher) (sqlPredicate, bool) {
	if len(matchers) == 0 {
		return sqlPredicate{}, false
	}

	parts := make([]string, 0, len(matchers))
	var args []any
	for i := range matchers {
		p, ok := translateMatcher(&matchers[i])
		if !ok {
			return sqlPredicate{}, false
		}
		parts = append(parts, p.where)
		args = append(args, p.args...)
	}

	joiner := " AND "
	if mode == "Or" {
		joiner = " OR "
	}
	return sqlPredicate{where: "(" + strings.Join(parts, joiner) + ")", args: args}, true
}

// translateMatcher translates a single matcher node - a value matcher or a
// nested group - or reports ok=false if it can't be safely translated.
func translateMatcher(m *library.ComicBookMatcher) (sqlPredicate, bool) {
	// Negation makes "safe superset" reasoning break down (a superset of
	// the positive predicate is a SUBSET, not a superset, of the negated
	// one) - always fall back rather than get this wrong.
	if m.Not {
		return sqlPredicate{}, false
	}

	if strings.Contains(m.Type, "GroupMatcher") {
		mode := m.MatcherMode
		if mode == "" {
			mode = "And"
		}
		return translateMatchers(mode, m.Matchers)
	}

	col, ok := matcherSQLColumns[library.MatcherType(m.Type)]
	if !ok {
		return sqlPredicate{}, false
	}

	switch col.kind {
	case kindString:
		return translateStringMatcher(col.column, m)
	case kindNumeric:
		return translateNumericMatcher(col.column, m)
	case kindBoolYesNo:
		return translateBoolYesNoMatcher(col.column, m)
	case kindEnumYesNo:
		return translateEnumYesNoMatcher(col.column, m)
	default:
		return sqlPredicate{}, false
	}
}

// parseOperator parses m.MatchOperator, defaulting to Equals for an empty
// string (matching how the rest of the codebase treats a blank operator).
func parseOperator(raw string) (library.MatchOperator, bool) {
	if raw == "" {
		return library.OperatorEquals, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return library.MatchOperator(n), true
}

func translateStringMatcher(column string, m *library.ComicBookMatcher) (sqlPredicate, bool) {
	op, ok := parseOperator(m.MatchOperator)
	if !ok {
		return sqlPredicate{}, false
	}

	switch op {
	case library.OperatorEquals:
		return sqlPredicate{where: column + " = ? COLLATE NOCASE", args: []any{m.MatchValue}}, true
	case library.OperatorContains:
		return sqlPredicate{where: column + " LIKE ? ESCAPE '\\' COLLATE NOCASE", args: []any{"%" + escapeLikePattern(m.MatchValue) + "%"}}, true
	case library.OperatorStartsWith:
		return sqlPredicate{where: column + " LIKE ? ESCAPE '\\' COLLATE NOCASE", args: []any{escapeLikePattern(m.MatchValue) + "%"}}, true
	case library.OperatorEndsWith:
		return sqlPredicate{where: column + " LIKE ? ESCAPE '\\' COLLATE NOCASE", args: []any{"%" + escapeLikePattern(m.MatchValue)}}, true
	default:
		// ContainsAny/ContainsAll/ListContains/Regex have no safe single-LIKE
		// translation - fall back.
		return sqlPredicate{}, false
	}
}

func translateNumericMatcher(column string, m *library.ComicBookMatcher) (sqlPredicate, bool) {
	op, ok := parseOperator(m.MatchOperator)
	if !ok {
		return sqlPredicate{}, false
	}

	// matchNumeric treats an empty MatchValue as "field unset" (0 or -1),
	// a special case not worth replicating in SQL - fall back.
	if m.MatchValue == "" {
		return sqlPredicate{}, false
	}
	v1, err := strconv.ParseFloat(m.MatchValue, 64)
	if err != nil {
		return sqlPredicate{}, false
	}

	switch op {
	case library.OperatorEquals:
		return sqlPredicate{where: column + " = ?", args: []any{v1}}, true
	case library.OperatorGreater:
		return sqlPredicate{where: column + " > ?", args: []any{v1}}, true
	case library.OperatorLesser:
		return sqlPredicate{where: column + " < ?", args: []any{v1}}, true
	case library.OperatorInRange:
		v2, err := strconv.ParseFloat(m.MatchValue2, 64)
		if err != nil {
			return sqlPredicate{}, false
		}
		return sqlPredicate{where: "(" + column + " >= ? AND " + column + " <= ?)", args: []any{v1, v2}}, true
	default:
		return sqlPredicate{}, false
	}
}

// translateBoolYesNoMatcher handles matchers backed by a native 0/1 boolean
// column (Checked, IsMissing) - these never take on an "Unknown" value, so
// only Yes/No are translated; Unknown falls back.
func translateBoolYesNoMatcher(column string, m *library.ComicBookMatcher) (sqlPredicate, bool) {
	op, ok := parseOperator(m.MatchOperator)
	if !ok {
		return sqlPredicate{}, false
	}
	switch op {
	case library.OperatorEqualsYes:
		return sqlPredicate{where: column + " = 1"}, true
	case library.OperatorEqualsNo:
		return sqlPredicate{where: column + " = 0"}, true
	default:
		return sqlPredicate{}, false
	}
}

// translateEnumYesNoMatcher handles matchers backed by a text column that
// stores "Yes"/"No"/"Unknown" (or empty, treated as Unknown) - SeriesComplete,
// BlackAndWhite.
func translateEnumYesNoMatcher(column string, m *library.ComicBookMatcher) (sqlPredicate, bool) {
	op, ok := parseOperator(m.MatchOperator)
	if !ok {
		return sqlPredicate{}, false
	}
	switch op {
	case library.OperatorEqualsYes:
		return sqlPredicate{where: column + " = ? COLLATE NOCASE", args: []any{"Yes"}}, true
	case library.OperatorEqualsNo:
		return sqlPredicate{where: column + " = ? COLLATE NOCASE", args: []any{"No"}}, true
	case library.OperatorEqualsUnknown:
		return sqlPredicate{where: "(" + column + " IS NULL OR " + column + " = '' OR " + column + " = ? COLLATE NOCASE)", args: []any{"Unknown"}}, true
	default:
		return sqlPredicate{}, false
	}
}

// escapeLikePattern escapes SQLite LIKE wildcards (% and _) plus the escape
// character itself in a value that's about to be embedded between % or
// used with LIKE, so literal % / _ in the matcher's value don't act as
// wildcards. Uses backslash as the escape character (paired with
// "ESCAPE '\\'" in the query).
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}
