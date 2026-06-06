package library

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MatchOperator represents the comparison operation for a matcher
type MatchOperator int

const (
	// Universal operators (used by all types)
	OperatorEquals MatchOperator = 0

	// String operators (ComicRack string matcher values)
	OperatorContains     MatchOperator = 1
	OperatorContainsAny  MatchOperator = 2
	OperatorContainsAll  MatchOperator = 3
	OperatorStartsWith   MatchOperator = 4
	OperatorEndsWith     MatchOperator = 5
	OperatorListContains MatchOperator = 6 // Field is a comma-separated list; checks membership
	OperatorRegex        MatchOperator = 7

	// Numeric operators (ComicRack numeric matcher values - reuse 1, 2, 3)
	OperatorGreater MatchOperator = 1 // Same as OperatorContains
	OperatorLesser  MatchOperator = 2 // Same as OperatorContainsAny
	OperatorInRange MatchOperator = 3 // Same as OperatorContainsAll

	// Date operators (ComicRack date matcher values - reuse constants)
	OperatorIsAfter       MatchOperator = 1 // Same as Greater
	OperatorIsBefore      MatchOperator = 2 // Same as Lesser
	OperatorIsInLastDays  MatchOperator = 3 // Same as InRange
	OperatorIsInDateRange MatchOperator = 4 // Same as StartsW ith

	// Yes/No operators (ComicRack yes/no matcher values)
	OperatorEqualsYes     MatchOperator = 0 // Same as Equals
	OperatorEqualsNo      MatchOperator = 1 // Same as Contains/Greater
	OperatorEqualsUnknown MatchOperator = 2 // Same as ContainsAny/Lesser
)

// MatcherType represents the property to match against
type MatcherType string

const (
	// Common matcher types
	MatcherTypeSeries          MatcherType = "Series"
	MatcherTypeAlternateSeries MatcherType = "AlternateSeries"
	MatcherTypeSeriesGroup     MatcherType = "SeriesGroup"
	MatcherTypeTitle           MatcherType = "Title"
	MatcherTypePublisher       MatcherType = "Publisher"
	MatcherTypeImprint         MatcherType = "Imprint"
	MatcherTypeWriter          MatcherType = "Writer"
	MatcherTypeYear            MatcherType = "Year"
	MatcherTypeMonth           MatcherType = "Month"
	MatcherTypeGenre           MatcherType = "Genre"
	MatcherTypeFormat          MatcherType = "Format"
	MatcherTypeCharacters      MatcherType = "Characters"
	MatcherTypeTeams           MatcherType = "Teams"
	MatcherTypeVolume          MatcherType = "Volume"
	MatcherTypeNumber          MatcherType = "Number"
	MatcherTypePageCount       MatcherType = "PageCount"
	MatcherTypeRating          MatcherType = "Rating"
	MatcherTypeTags            MatcherType = "Tags"
	MatcherTypeNotes           MatcherType = "Notes"
	MatcherTypeAddedTime       MatcherType = "Added"
	MatcherTypeOpenedTime      MatcherType = "Opened"
	MatcherTypeSeriesComplete  MatcherType = "SeriesComplete"
	MatcherTypeChecked         MatcherType = "Checked"
	MatcherTypeBlackAndWhite   MatcherType = "BlackAndWhite"
	MatcherTypeHasCustomValues MatcherType = "HasCustomValues"
	MatcherTypeIsLinked        MatcherType = "IsLinked"
	MatcherTypeIsMissing       MatcherType = "IsMissing"
	MatcherTypeModifiedInfo    MatcherType = "ModifiedInfo"
	MatcherTypeColorist        MatcherType = "Colorist"
	MatcherTypeCoverArtist     MatcherType = "CoverArtist"
	MatcherTypeEditor          MatcherType = "Editor"
	MatcherTypeInker           MatcherType = "Inker"
	MatcherTypeLetterer        MatcherType = "Letterer"
	MatcherTypePenciller       MatcherType = "Penciller"
	MatcherTypeTranslator          MatcherType = "Translator"
	MatcherTypeMainCharacterOrTeam MatcherType = "MainCharacterOrTeam"
	MatcherTypeLocations           MatcherType = "Locations"
	MatcherTypeStoryArc            MatcherType = "StoryArc"
	MatcherTypeAgeRating           MatcherType = "AgeRating"
	MatcherTypeSummary             MatcherType = "Summary"
	MatcherTypeReview              MatcherType = "Review"
	MatcherTypeLanguage        MatcherType = "LanguageISO"
	MatcherTypeDirectory       MatcherType = "Directory"
	MatcherTypeFile            MatcherType = "File"
	MatcherTypeFullPath        MatcherType = "FullPath"
	MatcherTypeFileFormat      MatcherType = "FileFormat"
	MatcherTypeScanInfo        MatcherType = "ScanInformation"
	MatcherTypeCount           MatcherType = "Count"
)

// Matcher represents a smart list filter rule
type Matcher struct {
	Type          MatcherType
	Operator      MatchOperator
	MatchValue    string
	MatchValue2   string // For range operators
	Not           bool   // Invert the match result
	IgnoreCase    bool   // For string comparisons
	compiledRegex *regexp.Regexp
}

// extractFieldName extracts the field name from a ComicRack matcher type
// e.g., "ComicBookDirectoryMatcher" -> "Directory"
//
//	"ComicBookSeriesMatcher" -> "Series"
func extractFieldName(matcherType string) string {
	// Remove "ComicBook" prefix and "Matcher" suffix
	name := strings.TrimPrefix(matcherType, "ComicBook")
	name = strings.TrimSuffix(name, "Matcher")
	return name
}

// NewMatcherFromXML creates a Matcher from XML ComicBookMatcher data
func NewMatcherFromXML(xmlMatcher *ComicBookMatcher) (*Matcher, error) {
	// Check if this is a group matcher (has nested matchers)
	if strings.Contains(xmlMatcher.Type, "GroupMatcher") {
		// Group matchers don't have MatchOperator/MatchValue, skip them
		// The nested matchers will be processed separately
		return nil, fmt.Errorf("group matcher not supported as individual matcher")
	}

	// Extract field name from matcher type
	// e.g., "ComicBookDirectoryMatcher" -> "Directory"
	fieldName := extractFieldName(xmlMatcher.Type)

	// Special handling for CustomValues matchers
	// CustomValues uses both MatchValue (key name) and MatchValue2 (expected value)
	if fieldName == "CustomValues" {
		// CustomValues matcher: key (MatchValue) must equal value (MatchValue2)
		return &Matcher{
			Type:        "CustomValues",
			MatchValue:  xmlMatcher.MatchValue,  // Key name
			MatchValue2: xmlMatcher.MatchValue2, // Expected value
			Not:         xmlMatcher.Not,
			IgnoreCase:  true,
			Operator:    OperatorEquals,
		}, nil
	}

	matchValue := xmlMatcher.MatchValue
	// Normalize path separators for file/directory matchers so Windows paths
	// stored in the library XML work correctly when the server runs on Linux.
	switch MatcherType(fieldName) {
	case MatcherTypeDirectory, MatcherTypeFile, MatcherTypeFullPath:
		matchValue = strings.ReplaceAll(matchValue, "\\", "/")
	}

	m := &Matcher{
		Type:        MatcherType(fieldName),
		MatchValue:  matchValue,
		MatchValue2: xmlMatcher.MatchValue2, // Needed for InRange / IsInDateRange operators
		Not:         xmlMatcher.Not,
		IgnoreCase:  true, // Default to case-insensitive
	}

	// Parse operator from XML MatchOperator attribute
	if err := m.parseOperator(xmlMatcher.MatchOperator); err != nil {
		return nil, fmt.Errorf("invalid operator %q: %w", xmlMatcher.MatchOperator, err)
	}

	return m, nil
}

// parseOperator converts the XML operator string/number to MatchOperator
func (m *Matcher) parseOperator(op string) error {
	// If operator is empty or missing, default to Equals (0)
	if op == "" {
		m.Operator = OperatorEquals
		return nil
	}

	// Try parsing as integer first (common in XML)
	if opNum, err := strconv.Atoi(op); err == nil {
		m.Operator = MatchOperator(opNum)

		// Compile regex if operator is OperatorRegex (7)
		if m.Operator == OperatorRegex {
			rx, err := regexp.Compile(m.MatchValue)
			if err != nil {
				return fmt.Errorf("invalid regex %q: %w", m.MatchValue, err)
			}
			m.compiledRegex = rx
		}
		return nil
	}

	// Parse string operator names
	switch strings.ToLower(op) {
	case "equal", "equals", "is":
		m.Operator = OperatorEquals
	case "contains":
		m.Operator = OperatorContains
	case "containsany":
		m.Operator = OperatorContainsAny
	case "containsall":
		m.Operator = OperatorContainsAll
	case "startswith":
		m.Operator = OperatorStartsWith
	case "endswith":
		m.Operator = OperatorEndsWith
	case "regex":
		m.Operator = OperatorRegex
	case "greater", "greaterthan":
		m.Operator = OperatorGreater
	case "lesser", "lessthan", "smaller":
		m.Operator = OperatorLesser
	case "inrange", "between":
		m.Operator = OperatorInRange
	case "isafter", "after":
		m.Operator = OperatorIsAfter
	case "isbefore", "before":
		m.Operator = OperatorIsBefore
	case "isinlastdays", "recent":
		m.Operator = OperatorIsInLastDays
	default:
		return fmt.Errorf("unknown operator: %s", op)
	}

	// Compile regex if needed
	if m.Operator == OperatorRegex {
		rx, err := regexp.Compile(m.MatchValue)
		if err != nil {
			return fmt.Errorf("invalid regex %q: %w", m.MatchValue, err)
		}
		m.compiledRegex = rx
	}

	return nil
}

// Match evaluates this matcher against a comic book
func (m *Matcher) Match(book *ComicBook) bool {
	result := m.matchInternal(book)

	// Apply negation if set
	if m.Not {
		return !result
	}
	return result
}

// matchInternal performs the actual matching logic
func (m *Matcher) matchInternal(book *ComicBook) bool {
	// Special handling for CustomValues matcher
	if m.Type == "CustomValues" {
		// CustomValuesStore format: ",key1=value1,key2=value2"
		// Mirrors ComicRack's GetCustomValue which returns null for missing keys.
		// null is coerced to "" before comparison, so a missing key == "".
		// "Custom Value key is (blank)" is true when key is absent OR has empty value.
		// NOT of that → true only when key has a non-empty value.
		store := book.CustomValuesStore
		key := m.MatchValue
		expectedValue := m.MatchValue2

		// Find the actual stored value for this key; nil means key absent.
		var found *string
		for _, pair := range strings.Split(store, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			parts := strings.SplitN(pair, "=", 2)
			pairKey := strings.TrimSpace(parts[0])
			keyMatch := (m.IgnoreCase && strings.EqualFold(pairKey, key)) || (!m.IgnoreCase && pairKey == key)
			if !keyMatch {
				continue
			}
			val := ""
			if len(parts) == 2 {
				val = strings.TrimSpace(parts[1])
			}
			found = &val
			break
		}

		// Coerce nil (absent key) to "" — mirrors ComicRack's null coercion.
		actualValue := ""
		if found != nil {
			actualValue = *found
		}

		if m.IgnoreCase {
			return strings.EqualFold(actualValue, expectedValue)
		}
		return actualValue == expectedValue
	}

	// Extract the value from the book based on matcher type
	value := m.getValue(book)

	// Perform type-specific comparison
	switch m.Type {
	case MatcherTypeYear, MatcherTypeMonth, MatcherTypeVolume,
		MatcherTypePageCount, MatcherTypeRating, MatcherTypeCount:
		return m.matchNumeric(value)
	case MatcherTypeAddedTime, MatcherTypeOpenedTime:
		return m.matchDate(value)
	case MatcherTypeSeriesComplete, MatcherTypeBlackAndWhite,
		MatcherTypeChecked, MatcherTypeHasCustomValues,
		MatcherTypeIsLinked, MatcherTypeIsMissing, MatcherTypeModifiedInfo:
		return m.matchYesNo(value)
	default:
		// String matching for all other types
		return m.matchString(value)
	}
}

// getValue extracts the property value from a ComicBook based on matcher type
func (m *Matcher) getValue(book *ComicBook) string {
	switch m.Type {
	case MatcherTypeSeries:
		return book.Series
	case MatcherTypeAlternateSeries:
		return book.AlternateSeries
	case MatcherTypeSeriesGroup:
		return book.SeriesGroup
	case MatcherTypeTitle:
		return book.Title
	case MatcherTypePublisher:
		return book.Publisher
	case MatcherTypeImprint:
		return book.Imprint
	case MatcherTypeWriter:
		return book.Writer
	case MatcherTypeColorist:
		return book.Colorist
	case MatcherTypeCoverArtist:
		return book.CoverArtist
	case MatcherTypeEditor:
		return book.Editor
	case MatcherTypeInker:
		return book.Inker
	case MatcherTypeLetterer:
		return book.Letterer
	case MatcherTypePenciller:
		return book.Penciller
	case MatcherTypeTranslator:
		return book.Translator
	case MatcherTypeMainCharacterOrTeam:
		return book.MainCharacterOrTeam
	case MatcherTypeLocations:
		return book.Locations
	case MatcherTypeStoryArc:
		return book.StoryArc
	case MatcherTypeAgeRating:
		return book.AgeRating
	case MatcherTypeSummary:
		return book.Summary
	case MatcherTypeReview:
		return book.Review
	case MatcherTypeYear:
		return strconv.Itoa(book.Year)
	case MatcherTypeMonth:
		return strconv.Itoa(book.Month)
	case MatcherTypeGenre:
		return book.Genre
	case MatcherTypeFormat:
		return book.Format
	case MatcherTypeCharacters:
		return book.Characters
	case MatcherTypeTeams:
		return book.Teams
	case MatcherTypeVolume:
		return strconv.Itoa(book.Volume)
	case MatcherTypeNumber:
		return book.Number
	case MatcherTypePageCount:
		return strconv.Itoa(book.PageCount)
	case MatcherTypeRating:
		return fmt.Sprintf("%.1f", book.Rating)
	case MatcherTypeTags:
		return book.Tags
	case MatcherTypeNotes:
		return book.Notes
	case MatcherTypeSeriesComplete:
		return book.SeriesComplete
	case MatcherTypeBlackAndWhite:
		return book.BlackAndWhite
	case MatcherTypeChecked:
		if book.Checked {
			return "yes"
		}
		return "no"
	case MatcherTypeHasCustomValues:
		if book.CustomValuesStore != "" {
			return "yes"
		}
		return "no"
	case MatcherTypeIsLinked:
		if book.FilePath != "" {
			return "yes"
		}
		return "no"
	case MatcherTypeIsMissing:
		if book.FileIsMissing {
			return "yes"
		}
		return "no"
	case MatcherTypeModifiedInfo:
		if book.ComicInfoIsDirty {
			return "yes"
		}
		return "no"
	case MatcherTypeLanguage:
		return book.LanguageISO
	case MatcherTypeAddedTime:
		return book.AddedTime.Time.Format(time.RFC3339)
	case MatcherTypeOpenedTime:
		return book.OpenedTime.Time.Format(time.RFC3339)
	case MatcherTypeDirectory:
		if book.FilePath != "" {
			// Normalize to forward slashes and use string ops only.
			// filepath.Dir on Windows re-introduces backslashes even when given
			// forward-slash input, breaking cross-platform path comparisons.
			normalized := strings.ReplaceAll(book.FilePath, "\\", "/")
			if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
				return normalized[:idx]
			}
			return ""
		}
		return ""
	case MatcherTypeFile:
		if book.FilePath != "" {
			// Normalize and extract filename without extension via string ops.
			// ComicRack uses Path.GetFileNameWithoutExtension (no extension).
			normalized := strings.ReplaceAll(book.FilePath, "\\", "/")
			base := normalized
			if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
				base = normalized[idx+1:]
			}
			if idx := strings.LastIndex(base, "."); idx > 0 {
				return base[:idx]
			}
			return base
		}
		return ""
	case MatcherTypeFullPath:
		return strings.ReplaceAll(book.FilePath, "\\", "/")
	case MatcherTypeFileFormat:
		// Extract file format from extension
		// e.g., ".cbz" -> "ZIP", ".cbr" -> "RAR"
		if book.FilePath != "" {
			ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(book.FilePath), "."))
			// Map common extensions to format names
			switch ext {
			case "CBZ", "ZIP":
				return "ZIP"
			case "CBR", "RAR":
				return "RAR"
			case "CB7", "7Z":
				return "7Z"
			case "CBT", "TAR":
				return "TAR"
			default:
				return ext
			}
		}
		return ""
	case MatcherTypeScanInfo:
		return book.ScanInformation
	case MatcherTypeCount:
		return strconv.Itoa(book.Count)
	default:
		return ""
	}
}

// matchString performs string-based comparison
func (m *Matcher) matchString(value string) bool {
	matchValue := m.MatchValue

	// Apply case sensitivity
	comp := func(a, b string) int {
		if m.IgnoreCase {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		}
		return strings.Compare(a, b)
	}

	// For regex, use the original value (don't case-fold)
	if m.Operator == OperatorRegex {
		if m.compiledRegex != nil {
			return m.compiledRegex.MatchString(value)
		}
		return false
	}

	// Apply case folding for other operators
	if m.IgnoreCase {
		value = strings.ToLower(value)
		matchValue = strings.ToLower(matchValue)
	}

	switch m.Operator {
	case OperatorEquals:
		return comp(value, matchValue) == 0
	case OperatorContains:
		return strings.Contains(value, matchValue)
	case OperatorContainsAny:
		// Split by comma or semicolon
		parts := strings.FieldsFunc(matchValue, func(r rune) bool {
			return r == ',' || r == ';'
		})
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.Contains(value, part) {
				return true
			}
		}
		return false
	case OperatorContainsAll:
		parts := strings.FieldsFunc(matchValue, func(r rune) bool {
			return r == ',' || r == ';'
		})
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !strings.Contains(value, part) {
				return false
			}
		}
		return true
	case OperatorStartsWith:
		return strings.HasPrefix(value, matchValue)
	case OperatorEndsWith:
		return strings.HasSuffix(value, matchValue)
	case OperatorListContains:
		// The field is a comma/semicolon-separated list; check if matchValue is a member.
		items := strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';'
		})
		for _, item := range items {
			if comp(strings.TrimSpace(item), matchValue) == 0 {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// matchNumeric performs numeric comparison
func (m *Matcher) matchNumeric(value string) bool {
	numValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}

	// Empty MatchValue means "field is not set" — represented as 0 in our model
	// (ComicRack uses 0/-1 as sentinels for unset numeric fields).
	if m.MatchValue == "" {
		return numValue == 0 || numValue == -1
	}

	matchNum, err := strconv.ParseFloat(m.MatchValue, 64)
	if err != nil {
		return false
	}

	switch m.Operator {
	case OperatorEquals:
		return numValue == matchNum
	case OperatorGreater:
		return numValue > matchNum
	case OperatorLesser:
		return numValue < matchNum
	case OperatorInRange:
		matchNum2, err := strconv.ParseFloat(m.MatchValue2, 64)
		if err != nil {
			return false
		}
		return numValue >= matchNum && numValue <= matchNum2
	default:
		return false
	}
}

// matchDate performs date/time comparison
func (m *Matcher) matchDate(value string) bool {
	dateValue, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return false
	}

	// Skip zero times (unset dates)
	if dateValue.IsZero() {
		return false
	}

	// For OperatorIsInLastDays, we don't parse matchValue as a date
	if m.Operator == OperatorIsInLastDays || m.Operator == OperatorIsInDateRange {
		// These operators don't use matchValue as a date, skip parsing
	} else {
		matchDate, err := time.Parse(time.RFC3339, m.MatchValue)
		if err != nil {
			return false
		}
		_ = matchDate // Will be used in switch below
	}

	matchDate, err := time.Parse(time.RFC3339, m.MatchValue)
	if err != nil && m.Operator != OperatorIsInLastDays && m.Operator != OperatorIsInDateRange {
		return false
	}

	switch m.Operator {
	case OperatorEquals:
		return dateValue.Equal(matchDate)
	case OperatorIsAfter:
		return dateValue.After(matchDate)
	case OperatorIsBefore:
		return dateValue.Before(matchDate)
	case OperatorIsInLastDays:
		days, err := strconv.Atoi(m.MatchValue)
		if err != nil {
			return false
		}
		now := time.Now()
		cutoff := now.AddDate(0, 0, -days)
		// dateValue must be after or equal to the cutoff AND not in the future
		return (dateValue.After(cutoff) || dateValue.Equal(cutoff)) && (dateValue.Before(now) || dateValue.Equal(now))
	case OperatorIsInDateRange:
		matchDate2, err := time.Parse(time.RFC3339, m.MatchValue2)
		if err != nil {
			return false
		}
		return (dateValue.After(matchDate) || dateValue.Equal(matchDate)) &&
			(dateValue.Before(matchDate2) || dateValue.Equal(matchDate2))
	default:
		return false
	}
}

// matchYesNo performs Yes/No/Unknown comparison
func (m *Matcher) matchYesNo(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))

	switch m.Operator {
	case OperatorEqualsYes:
		return value == "yes"
	case OperatorEqualsNo:
		return value == "no"
	case OperatorEqualsUnknown:
		return value == "unknown" || value == ""
	default:
		return false
	}
}

// evaluateMatcher recursively evaluates a matcher (value or group) against a book
func evaluateMatcher(xmlMatcher *ComicBookMatcher, book *ComicBook) bool {
	// Check if this is a group matcher
	if strings.Contains(xmlMatcher.Type, "GroupMatcher") {
		// Evaluate nested matchers according to group mode
		groupMode := xmlMatcher.MatcherMode
		if groupMode == "" {
			groupMode = "And" // Default
		}

		if len(xmlMatcher.Matchers) == 0 {
			// Empty group matcher - no conditions to evaluate
			// This should NOT match anything (return false for AND, true for OR to maintain neutral element)
			// But in practice, lists with empty group matchers in ComicRack use CacheStorage
			// For safety, treat as no match
			return false
		}

		var result bool
		if groupMode == "Or" {
			// OR mode: true if ANY nested matcher matches
			result = false
			for i := range xmlMatcher.Matchers {
				if evaluateMatcher(&xmlMatcher.Matchers[i], book) {
					result = true
					break
				}
			}
		} else {
			// AND mode: true if ALL nested matchers match
			result = true
			for i := range xmlMatcher.Matchers {
				if !evaluateMatcher(&xmlMatcher.Matchers[i], book) {
					result = false
					break
				}
			}
		}

		// Apply negation if set
		if xmlMatcher.Not {
			return !result
		}
		return result
	}

	// Regular value matcher
	matcher, err := NewMatcherFromXML(xmlMatcher)
	if err != nil {
		// Unsupported matcher type - log and return false (don't match)
		// Common unsupported types: CustomValues, Plugin, Expression, ReadPercentage, etc.
		// These require additional implementation
		// Silently return false to avoid flooding logs
		return false
	}

	return matcher.Match(book)
}

// matchBooks evaluates a list's matchers against a slice of candidate books.
func matchBooks(list *ComicListItem, candidates []*ComicBook) []*ComicBook {
	matcherMode := list.MatcherMode
	if matcherMode == "" {
		matcherMode = "And"
	}

	var matched []*ComicBook
	for _, book := range candidates {
		var ok bool
		if matcherMode == "Or" {
			for j := range list.Matchers {
				if evaluateMatcher(&list.Matchers[j], book) {
					ok = true
					break
				}
			}
		} else {
			ok = true
			for j := range list.Matchers {
				if !evaluateMatcher(&list.Matchers[j], book) {
					ok = false
					break
				}
			}
		}
		if ok {
			matched = append(matched, book)
		}
	}
	return matched
}

// MatchBooks evaluates a smart list against the library.
// If the list has a BaseListId, candidates are first restricted to books
// matching that base list (mirrors ComicRack's "Match on [source]" scope).
func (l *ComicLibrary) MatchBooks(list *ComicListItem) ([]*ComicBook, error) {
	if list == nil {
		return nil, fmt.Errorf("list is nil")
	}
	if !strings.Contains(list.Type, "SmartList") {
		return nil, fmt.Errorf("list %q is not a smart list (type: %s)", list.Name, list.Type)
	}
	if len(list.Matchers) == 0 {
		return nil, fmt.Errorf("no matchers in list %q", list.Name)
	}

	// Build the candidate set.
	var candidates []*ComicBook
	if list.BaseListId != "" {
		baseList := l.FindListByID(list.BaseListId)
		if baseList == nil {
			// Base list not found — treat as empty scope.
			return nil, nil
		}
		if strings.Contains(baseList.Type, "SmartList") {
			// Recursively evaluate the base smart list.
			var err error
			candidates, err = l.MatchBooks(baseList)
			if err != nil {
				return nil, fmt.Errorf("evaluating base list %q: %w", baseList.Name, err)
			}
		} else {
			// Reading list — use the referenced books directly.
			candidates = l.GetBooksByList(baseList)
		}
	} else {
		candidates = make([]*ComicBook, len(l.Books))
		for i := range l.Books {
			candidates[i] = &l.Books[i]
		}
	}

	return matchBooks(list, candidates), nil
}

// GetBooksForList returns books for any list type: SmartList (matcher-based),
// IdList (explicit GUID set), or ReadingList (ordered book references).
func (l *ComicLibrary) GetBooksForList(list *ComicListItem) ([]*ComicBook, error) {
	if list == nil {
		return nil, fmt.Errorf("list is nil")
	}
	if strings.Contains(list.Type, "SmartList") {
		return l.MatchBooks(list)
	}
	if strings.Contains(list.Type, "IdListItem") {
		books := make([]*ComicBook, 0, len(list.BookIds))
		for _, id := range list.BookIds {
			if book := l.GetBook(id); book != nil {
				books = append(books, book)
			}
		}
		return books, nil
	}
	// ComicReadingList and anything else — use Items references
	return l.GetBooksByList(list), nil
}
