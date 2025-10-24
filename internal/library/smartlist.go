package library

import (
	"fmt"
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
	OperatorContains    MatchOperator = 1
	OperatorContainsAny MatchOperator = 2
	OperatorContainsAll MatchOperator = 3
	OperatorStartsWith  MatchOperator = 4
	OperatorEndsWith    MatchOperator = 5
	// 6 is ListContains (not implemented yet)
	OperatorRegex MatchOperator = 7

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
	MatcherTypeSeries         MatcherType = "Series"
	MatcherTypeTitle          MatcherType = "Title"
	MatcherTypePublisher      MatcherType = "Publisher"
	MatcherTypeWriter         MatcherType = "Writer"
	MatcherTypeYear           MatcherType = "Year"
	MatcherTypeMonth          MatcherType = "Month"
	MatcherTypeGenre          MatcherType = "Genre"
	MatcherTypeFormat         MatcherType = "Format"
	MatcherTypeCharacters     MatcherType = "Characters"
	MatcherTypeVolume         MatcherType = "Volume"
	MatcherTypeNumber         MatcherType = "Number"
	MatcherTypePageCount      MatcherType = "PageCount"
	MatcherTypeRating         MatcherType = "Rating"
	MatcherTypeTags           MatcherType = "Tags"
	MatcherTypeNotes          MatcherType = "Notes"
	MatcherTypeAddedTime      MatcherType = "Added"
	MatcherTypeOpenedTime     MatcherType = "Opened"
	MatcherTypeSeriesComplete MatcherType = "SeriesComplete"
	MatcherTypeLanguage       MatcherType = "LanguageISO"
	MatcherTypeImprint        MatcherType = "Imprint"
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

// NewMatcherFromXML creates a Matcher from XML ComicBookMatcher data
func NewMatcherFromXML(xmlMatcher *ComicBookMatcher) (*Matcher, error) {
	m := &Matcher{
		Type:       MatcherType(xmlMatcher.Type),
		MatchValue: xmlMatcher.ArgumentValue,
		IgnoreCase: true, // Default to case-insensitive
	}

	// Parse operator from XML Operator attribute
	if err := m.parseOperator(xmlMatcher.Operator); err != nil {
		return nil, fmt.Errorf("invalid operator %q: %w", xmlMatcher.Operator, err)
	}

	return m, nil
}

// parseOperator converts the XML operator string/number to MatchOperator
func (m *Matcher) parseOperator(op string) error {
	// Try parsing as integer first (common in XML)
	if opNum, err := strconv.Atoi(op); err == nil {
		m.Operator = MatchOperator(opNum)

		// Compile regex if operator is OperatorRegex (6)
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
	// Extract the value from the book based on matcher type
	value := m.getValue(book)

	// Perform type-specific comparison
	switch m.Type {
	case MatcherTypeYear, MatcherTypeMonth, MatcherTypeVolume,
		MatcherTypePageCount, MatcherTypeRating:
		return m.matchNumeric(value)
	case MatcherTypeAddedTime, MatcherTypeOpenedTime:
		return m.matchDate(value)
	case MatcherTypeSeriesComplete:
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
	case MatcherTypeTitle:
		return book.Title
	case MatcherTypePublisher:
		return book.Publisher
	case MatcherTypeWriter:
		return book.Writer
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
	case MatcherTypeLanguage:
		return book.LanguageISO
	case MatcherTypeImprint:
		return book.Imprint
	case MatcherTypeAddedTime:
		return book.AddedTime.Time.Format(time.RFC3339)
	case MatcherTypeOpenedTime:
		return book.OpenedTime.Time.Format(time.RFC3339)
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

// MatchBooks evaluates a smart list against a collection of books
// Returns books that match the smart list criteria
func (l *ComicLibrary) MatchBooks(list *ComicListItem) ([]*ComicBook, error) {
	if list == nil {
		return nil, fmt.Errorf("list is nil")
	}

	// Check if this is a smart list
	if !strings.Contains(list.Type, "SmartList") {
		return nil, fmt.Errorf("list %q is not a smart list (type: %s)", list.Name, list.Type)
	}

	// Convert XML matchers to our Matcher type
	matchers := make([]*Matcher, 0, len(list.Matchers))
	for i := range list.Matchers {
		matcher, err := NewMatcherFromXML(&list.Matchers[i])
		if err != nil {
			// Log warning but continue with other matchers
			continue
		}
		matchers = append(matchers, matcher)
	}

	if len(matchers) == 0 {
		return nil, fmt.Errorf("no valid matchers in list %q", list.Name)
	}

	// Determine matcher mode (AND vs OR)
	matcherMode := list.MatcherMode
	if matcherMode == "" {
		matcherMode = "And" // Default to AND
	}

	// Evaluate each book
	var matchedBooks []*ComicBook
	for i := range l.Books {
		book := &l.Books[i]

		if matcherMode == "Or" {
			// OR mode: book matches if ANY matcher matches
			for _, matcher := range matchers {
				if matcher.Match(book) {
					matchedBooks = append(matchedBooks, book)
					break
				}
			}
		} else {
			// AND mode: book matches if ALL matchers match
			allMatch := true
			for _, matcher := range matchers {
				if !matcher.Match(book) {
					allMatch = false
					break
				}
			}
			if allMatch {
				matchedBooks = append(matchedBooks, book)
			}
		}
	}

	return matchedBooks, nil
}
