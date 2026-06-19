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
	MatcherTypeSeries              MatcherType = "Series"
	MatcherTypeAlternateSeries     MatcherType = "AlternateSeries"
	MatcherTypeSeriesGroup         MatcherType = "SeriesGroup"
	MatcherTypeTitle               MatcherType = "Title"
	MatcherTypePublisher           MatcherType = "Publisher"
	MatcherTypeImprint             MatcherType = "Imprint"
	MatcherTypeWriter              MatcherType = "Writer"
	MatcherTypeYear                MatcherType = "Year"
	MatcherTypeMonth               MatcherType = "Month"
	MatcherTypeGenre               MatcherType = "Genre"
	MatcherTypeFormat              MatcherType = "Format"
	MatcherTypeCharacters          MatcherType = "Characters"
	MatcherTypeTeams               MatcherType = "Teams"
	MatcherTypeVolume              MatcherType = "Volume"
	MatcherTypeNumber              MatcherType = "Number"
	MatcherTypePageCount           MatcherType = "PageCount"
	MatcherTypeRating              MatcherType = "Rating"
	MatcherTypeTags                MatcherType = "Tags"
	MatcherTypeNotes               MatcherType = "Notes"
	MatcherTypeAddedTime           MatcherType = "Added"
	MatcherTypeOpenedTime          MatcherType = "Opened"
	MatcherTypeSeriesComplete      MatcherType = "SeriesComplete"
	MatcherTypeChecked             MatcherType = "Checked"
	MatcherTypeBlackAndWhite       MatcherType = "BlackAndWhite"
	MatcherTypeHasCustomValues     MatcherType = "HasCustomValues"
	MatcherTypeIsLinked            MatcherType = "IsLinked"
	MatcherTypeIsMissing           MatcherType = "IsMissing"
	MatcherTypeModifiedInfo        MatcherType = "ModifiedInfo"
	MatcherTypeColorist            MatcherType = "Colorist"
	MatcherTypeCoverArtist         MatcherType = "CoverArtist"
	MatcherTypeEditor              MatcherType = "Editor"
	MatcherTypeInker               MatcherType = "Inker"
	MatcherTypeLetterer            MatcherType = "Letterer"
	MatcherTypePenciller           MatcherType = "Penciller"
	MatcherTypeTranslator          MatcherType = "Translator"
	MatcherTypeMainCharacterOrTeam MatcherType = "MainCharacterOrTeam"
	MatcherTypeLocations           MatcherType = "Locations"
	MatcherTypeStoryArc            MatcherType = "StoryArc"
	MatcherTypeAgeRating           MatcherType = "AgeRating"
	MatcherTypeSummary             MatcherType = "Summary"
	MatcherTypeReview              MatcherType = "Review"
	MatcherTypeReadPercentage      MatcherType = "ReadPercentage"
	MatcherTypeManga               MatcherType = "Manga"
	MatcherTypeAllProperties       MatcherType = "AllProperties"
	// Miscellaneous string
	MatcherTypeISBN MatcherType = "ISBN"
	MatcherTypeWeb  MatcherType = "Web"
	// Book ownership/condition string
	MatcherTypeBookAge              MatcherType = "BookAge"
	MatcherTypeBookCondition        MatcherType = "BookCondition"
	MatcherTypeBookStore            MatcherType = "BookStore"
	MatcherTypeBookOwner            MatcherType = "BookOwner"
	MatcherTypeBookCollectionStatus MatcherType = "BookCollectionStatus"
	MatcherTypeBookNotes            MatcherType = "BookNotes"
	MatcherTypeBookLocation         MatcherType = "BookLocation"
	// Numeric
	MatcherTypeAlternateNumber MatcherType = "AlternateNumber"
	MatcherTypeAlternateCount  MatcherType = "AlternateCount"
	MatcherTypeFileSize        MatcherType = "FileSize"
	MatcherTypeCommunityRating MatcherType = "CommunityRating"
	// Date
	MatcherTypeModified      MatcherType = "Modified"
	MatcherTypeCreation      MatcherType = "Creation"
	MatcherTypePublished     MatcherType = "Published"
	MatcherTypeReleased      MatcherType = "Released"
	MatcherTypeDay           MatcherType = "Day"
	MatcherTypeWeek          MatcherType = "Week"
	MatcherTypeNewPages      MatcherType = "NewPages"
	MatcherTypeBookmarkCount MatcherType = "BookmarkCount"
	MatcherTypeBookPrice     MatcherType = "BookPrice"
	MatcherTypeLanguage      MatcherType = "LanguageISO"
	MatcherTypeDirectory     MatcherType = "Directory"
	MatcherTypeFile          MatcherType = "File"
	MatcherTypeFullPath      MatcherType = "FullPath"
	MatcherTypeFileFormat    MatcherType = "FileFormat"
	MatcherTypeScanInfo      MatcherType = "ScanInformation"
	MatcherTypeCount         MatcherType = "Count"

	// Duplicate matcher — requires cross-book comparison across the entire candidate set.
	// Maps to ComicBookDuplicateMatcher xsi:type in the ComicRack XML.
	// Operator 0 (On) returns books that are duplicates; operator != 0 (Off) returns all books.
	MatcherTypeDuplicate MatcherType = "Duplicate"

	// ComicVine enrichment matchers — comic-server only, not recognized by ComicRack.
	// Maps to ComicServerCV*Matcher xsi:type values.
	MatcherTypeCVSeriesComplete MatcherType = "CVSeriesComplete" // yes/no/unknown
	MatcherTypeCVMissingCount   MatcherType = "CVMissingCount"   // numeric: missing issue count
	MatcherTypeCVPercentOwned   MatcherType = "CVPercentOwned"   // numeric: 0-100

	// Series aggregate matchers — require stats across all books in the same series+volume.
	// These map to SmartListSeries*Matcher xsi:type values in the ComicRack XML.
	MatcherTypeSeriesAllComplete            MatcherType = "SeriesAllComplete"
	MatcherTypeSeriesAverageCommunityRating MatcherType = "SeriesAverageCommunityRating"
	MatcherTypeSeriesAverageRating          MatcherType = "SeriesAverageRating"
	MatcherTypeSeriesCount                  MatcherType = "SeriesCount"
	MatcherTypeSeriesEndOfGap               MatcherType = "SeriesEndOfGap"
	MatcherTypeSeriesFirstNumber            MatcherType = "SeriesFirstNumber"
	MatcherTypeSeriesGaps                   MatcherType = "SeriesGaps"
	MatcherTypeSeriesLastAddedTime          MatcherType = "SeriesLastAddedTime"
	MatcherTypeSeriesLastNumber             MatcherType = "SeriesLastNumber"
	MatcherTypeSeriesLastOpenedTime         MatcherType = "SeriesLastOpenedTime"
	MatcherTypeSeriesLastPublishedTime      MatcherType = "SeriesLastPublishedTime"
	MatcherTypeSeriesLastReleasedTime       MatcherType = "SeriesLastReleasedTime"
	MatcherTypeSeriesMaxCount               MatcherType = "SeriesMaxCount"
	MatcherTypeSeriesMaxGapSize             MatcherType = "SeriesMaxGapSize"
	MatcherTypeSeriesMaxYear                MatcherType = "SeriesMaxYear"
	MatcherTypeSeriesMinCount               MatcherType = "SeriesMinCount"
	MatcherTypeSeriesMinYear                MatcherType = "SeriesMinYear"
	MatcherTypeSeriesPageCount              MatcherType = "SeriesPageCount"
	MatcherTypeSeriesPagesRead              MatcherType = "SeriesPagesRead"
	MatcherTypeSeriesPercentRead            MatcherType = "SeriesPercentRead"
	MatcherTypeSeriesRunningTimeYears       MatcherType = "SeriesRunningTimeYears"
	MatcherTypeSeriesStartOfGap             MatcherType = "SeriesStartOfGap"
)

// AllPropertiesOption controls which fields the AllProperties matcher searches
type AllPropertiesOption string

const (
	AllPropertiesAll         AllPropertiesOption = "All"
	AllPropertiesSeries      AllPropertiesOption = "Series"
	AllPropertiesWriter      AllPropertiesOption = "Writer"
	AllPropertiesArtists     AllPropertiesOption = "Artists"
	AllPropertiesDescriptive AllPropertiesOption = "Descriptive"
	AllPropertiesFile        AllPropertiesOption = "File"
	AllPropertiesCatalog     AllPropertiesOption = "Catalog"
)

// Matcher represents a smart list filter rule
type Matcher struct {
	Type                MatcherType
	Operator            MatchOperator
	MatchValue          string
	MatchValue2         string              // For range operators
	Not                 bool                // Invert the match result
	IgnoreCase          bool                // For string comparisons
	AllPropertiesOption AllPropertiesOption // For MatcherTypeAllProperties only
	compiledRegex       *regexp.Regexp
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
// seriesMatcherTypes maps SmartListSeries*Matcher XML type names to MatcherType constants.
// Note: FirstNumber and LastNumber have XmlType aliases with a typo ("Numbert") in ComicRack CE.
var seriesMatcherTypes = map[string]MatcherType{
	"SmartListSeriesAllCompleteMatcher":            MatcherTypeSeriesAllComplete,
	"SmartListSeriesAverageCommunityRatingMatcher": MatcherTypeSeriesAverageCommunityRating,
	"SmartListSeriesAverageRatingMatcher":          MatcherTypeSeriesAverageRating,
	"SmartListSeriesCountMatcher":                  MatcherTypeSeriesCount,
	"SmartListSeriesEndOfGapMatcher":               MatcherTypeSeriesEndOfGap,
	"SmartListSeriesFirstNumberMatcher":            MatcherTypeSeriesFirstNumber,
	"SmartListSeriesMinNumbertMatcher":             MatcherTypeSeriesFirstNumber, // XmlType alias (typo in ComicRack CE)
	"SmartListSeriesGapsMatcher":                   MatcherTypeSeriesGaps,
	"SmartListSeriesLastAddedTimeMatcher":          MatcherTypeSeriesLastAddedTime,
	"SmartListSeriesLastNumberMatcher":             MatcherTypeSeriesLastNumber,
	"SmartListSeriesMaxNumbertMatcher":             MatcherTypeSeriesLastNumber, // XmlType alias (typo in ComicRack CE)
	"SmartListSeriesLastOpenedTimeMatcher":         MatcherTypeSeriesLastOpenedTime,
	"SmartListSeriesLastPublishedTimeMatcher":      MatcherTypeSeriesLastPublishedTime,
	"SmartListSeriesLastReleasedTimeMatcher":       MatcherTypeSeriesLastReleasedTime,
	"SmartListSeriesMaxCountMatcher":               MatcherTypeSeriesMaxCount,
	"SmartListSeriesMaxGapSizeMatcher":             MatcherTypeSeriesMaxGapSize,
	"SmartListSeriesMaxYearMatcher":                MatcherTypeSeriesMaxYear,
	"SmartListSeriesMinCountMatcher":               MatcherTypeSeriesMinCount,
	"SmartListSeriesMinYearMatcher":                MatcherTypeSeriesMinYear,
	"SmartListSeriesPageCountMatcher":              MatcherTypeSeriesPageCount,
	"SmartListSeriesPagesReadMatcher":              MatcherTypeSeriesPagesRead,
	"SmartListSeriesPercentReadMatcher":            MatcherTypeSeriesPercentRead,
	"SmartListSeriesRunningTimeYearsMatcher":       MatcherTypeSeriesRunningTimeYears,
	"SmartListSeriesStartOfGapMatcher":             MatcherTypeSeriesStartOfGap,
}

// isSeriesMatcherType returns true if the XML type name is a series aggregate matcher.
func isSeriesMatcherType(xmlType string) bool {
	_, ok := seriesMatcherTypes[xmlType]
	return ok
}

func NewMatcherFromXML(xmlMatcher *ComicBookMatcher) (*Matcher, error) {
	// Check if this is a group matcher (has nested matchers)
	if strings.Contains(xmlMatcher.Type, "GroupMatcher") {
		// Group matchers don't have MatchOperator/MatchValue, skip them
		// The nested matchers will be processed separately
		return nil, fmt.Errorf("group matcher not supported as individual matcher")
	}

	// Handle series aggregate matchers
	if mt, ok := seriesMatcherTypes[xmlMatcher.Type]; ok {
		m := &Matcher{
			Type:        mt,
			MatchValue:  xmlMatcher.MatchValue,
			MatchValue2: xmlMatcher.MatchValue2,
			Not:         xmlMatcher.Not,
			IgnoreCase:  true,
		}
		if err := m.parseOperator(xmlMatcher.MatchOperator); err != nil {
			return nil, fmt.Errorf("invalid operator %q: %w", xmlMatcher.MatchOperator, err)
		}
		return m, nil
	}

	// Handle ComicVine enrichment matchers
	if mt, ok := cvMatcherTypes[xmlMatcher.Type]; ok {
		m := &Matcher{
			Type:        mt,
			MatchValue:  xmlMatcher.MatchValue,
			MatchValue2: xmlMatcher.MatchValue2,
			Not:         xmlMatcher.Not,
			IgnoreCase:  true,
		}
		if err := m.parseOperator(xmlMatcher.MatchOperator); err != nil {
			return nil, fmt.Errorf("invalid operator %q: %w", xmlMatcher.MatchOperator, err)
		}
		return m, nil
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

	opt := AllPropertiesOption(xmlMatcher.Option)
	if opt == "" {
		opt = AllPropertiesAll
	}

	m := &Matcher{
		Type:                MatcherType(fieldName),
		MatchValue:          matchValue,
		MatchValue2:         xmlMatcher.MatchValue2,
		Not:                 xmlMatcher.Not,
		IgnoreCase:          true,
		AllPropertiesOption: opt,
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
		MatcherTypePageCount, MatcherTypeRating, MatcherTypeCount,
		MatcherTypeReadPercentage, MatcherTypeAlternateCount,
		MatcherTypeFileSize, MatcherTypeCommunityRating,
		MatcherTypeDay, MatcherTypeWeek,
		MatcherTypeNewPages, MatcherTypeBookmarkCount, MatcherTypeBookPrice:
		return m.matchNumeric(value)
	case MatcherTypeAlternateNumber:
		// AlternateNumber is stored as a string in ComicBook but matched numerically
		return m.matchNumeric(value)
	case MatcherTypeAddedTime, MatcherTypeOpenedTime,
		MatcherTypeModified, MatcherTypeCreation,
		MatcherTypePublished, MatcherTypeReleased:
		return m.matchDate(value)
	case MatcherTypeSeriesComplete, MatcherTypeBlackAndWhite,
		MatcherTypeChecked, MatcherTypeHasCustomValues,
		MatcherTypeIsLinked, MatcherTypeIsMissing, MatcherTypeModifiedInfo:
		return m.matchYesNo(value)
	case MatcherTypeManga:
		return m.matchManga(value)
	case MatcherTypeAllProperties:
		return m.matchAllProperties(book)
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
	// Miscellaneous string
	case MatcherTypeISBN:
		return book.ISBN
	case MatcherTypeWeb:
		return book.Web
	// Book ownership/condition
	case MatcherTypeBookAge:
		return book.BookAge
	case MatcherTypeBookCondition:
		return book.BookCondition
	case MatcherTypeBookStore:
		return book.BookStore
	case MatcherTypeBookOwner:
		return book.BookOwner
	case MatcherTypeBookCollectionStatus:
		return book.BookCollectionStatus
	case MatcherTypeBookNotes:
		return book.BookNotes
	case MatcherTypeBookLocation:
		return book.BookLocation
	// Numeric
	case MatcherTypeAlternateNumber:
		return book.AlternateNumber
	case MatcherTypeAlternateCount:
		return strconv.Itoa(book.AlternateCount)
	case MatcherTypeFileSize:
		return strconv.FormatInt(book.FileSize, 10)
	case MatcherTypeCommunityRating:
		return fmt.Sprintf("%.1f", book.CommunityRating)
	case MatcherTypeDay:
		return strconv.Itoa(book.Day)
	case MatcherTypeWeek:
		return bookWeekOfYear(book)
	case MatcherTypeNewPages:
		return strconv.Itoa(book.NewPages)
	case MatcherTypeBookmarkCount:
		count := 0
		for _, p := range book.Pages {
			if p.Bookmark != "" {
				count++
			}
		}
		return strconv.Itoa(count)
	case MatcherTypeBookPrice:
		if book.BookPrice < 0 {
			return ""
		}
		return fmt.Sprintf("%.2f", book.BookPrice)
	// Date
	case MatcherTypeModified:
		if book.FileModifiedTime.Time.IsZero() {
			return ""
		}
		return book.FileModifiedTime.Time.Format(time.RFC3339)
	case MatcherTypeCreation:
		if book.FileCreationTime.Time.IsZero() {
			return ""
		}
		return book.FileCreationTime.Time.Format(time.RFC3339)
	case MatcherTypePublished:
		// Published is computed from Year+Month+Day (mirrors ComicRackCE's Published property)
		if book.Year <= 0 {
			return ""
		}
		year := book.Year
		month := book.Month
		if month < 1 {
			month = 1
		}
		if month > 12 {
			month = 12
		}
		day := book.Day
		maxDay := daysInMonth(year, month)
		if day < 1 {
			day = 1
		}
		if day > maxDay {
			day = maxDay
		}
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	case MatcherTypeReleased:
		if book.ReleasedTime.Time.IsZero() {
			return ""
		}
		return book.ReleasedTime.Time.Format(time.RFC3339)
	case MatcherTypeReadPercentage:
		return strconv.Itoa(bookReadPercentage(book))
	case MatcherTypeManga:
		return book.Manga
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

// daysInMonth returns the number of days in the given month/year.
func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// bookWeekOfYear mirrors ComicRackCE's Week property: week-of-year derived from
// the Published date (Year+Month+Day). Returns -1 (as string) if no published date.
// Uses ISO week numbering (Monday = start of week), matching .NET's CalendarWeekRule.FirstDay
// with DayOfWeek.Monday for all but edge-case years.
func bookWeekOfYear(book *ComicBook) string {
	if book.Year <= 0 {
		return "-1"
	}
	month := book.Month
	if month < 1 {
		month = 1
	}
	if month > 12 {
		month = 12
	}
	day := book.Day
	if day < 1 {
		day = 1
	}
	maxDay := daysInMonth(book.Year, month)
	if day > maxDay {
		day = maxDay
	}
	_, week := time.Date(book.Year, time.Month(month), day, 0, 0, 0, 0, time.UTC).ISOWeek()
	return strconv.Itoa(week)
}

// bookReadPercentage mirrors ComicRackCE's ReadPercentage property:
// (LastPageRead+1)*100/PageCount clamped to [1,100], or 0 if unread/no pages.
func bookReadPercentage(book *ComicBook) int {
	if book.PageCount <= 0 || book.LastPageRead <= 0 {
		return 0
	}
	pct := (book.LastPageRead + 1) * 100 / book.PageCount
	if pct < 1 {
		return 1
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// matchManga performs the 4-way Manga enum comparison used by ComicRackCE.
// Operators: 0=is Yes (RTL), 1=is Yes (LTR), 2=is No, 3=is Unknown.
// The XML value for LTR manga is "YesRightToLeft" per ComicInfo spec;
// "YesAndRightToLeft" (C# enum name) is also accepted for safety.
func (m *Matcher) matchManga(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	switch m.Operator {
	case 0: // is Yes (right-to-left, traditional manga)
		return v == "yes"
	case 1: // is Yes, Left to Right
		return v == "yesrighttoleft" || v == "yesandrighttoleft"
	case 2: // is No
		return v == "no"
	case 3: // is Unknown
		return v == "unknown" || v == ""
	default:
		return false
	}
}

// allPropertiesFields returns the set of field values to search for a given option,
// mirroring ComicRackCE's ComicBookAllPropertiesMatcher.GetOptionValueSet.
// Fields from unimplemented issues (BookAge, BookNotes, etc.) return "" until added.
func allPropertiesFields(book *ComicBook, option AllPropertiesOption) []string {
	switch option {
	case AllPropertiesSeries:
		return []string{book.Series, book.AlternateSeries, book.Format, book.SeriesGroup, book.StoryArc}
	case AllPropertiesWriter:
		return []string{book.Writer}
	case AllPropertiesArtists:
		return []string{book.Writer, book.Penciller, book.Inker, book.Colorist,
			book.Editor, book.Translator, book.Letterer, book.CoverArtist}
	case AllPropertiesFile:
		return []string{strings.ReplaceAll(book.FilePath, "\\", "/")}
	case AllPropertiesDescriptive:
		return []string{book.Notes, book.Summary, book.Review, book.Tags,
			book.Characters, book.Teams, book.MainCharacterOrTeam, book.Locations, book.ScanInformation}
	case AllPropertiesCatalog:
		return []string{book.BookAge, book.BookCollectionStatus, book.BookNotes,
			book.BookOwner, book.BookStore, book.BookLocation, book.ISBN}
	default: // AllPropertiesAll
		vol := strconv.Itoa(book.Volume)
		year := strconv.Itoa(book.Year)
		return []string{
			book.Series, book.AlternateSeries, book.Title, book.SeriesGroup, book.StoryArc,
			book.Writer, book.Penciller, book.Inker, book.Colorist, book.Letterer,
			book.Editor, book.Translator, book.CoverArtist,
			book.Summary, strings.ReplaceAll(book.FilePath, "\\", "/"),
			book.Genre, book.Notes, book.Review, book.Publisher, book.Imprint,
			vol, book.Number, year, book.Format, book.AgeRating,
			book.Tags, book.Characters, book.Teams, book.MainCharacterOrTeam,
			book.Locations, book.ScanInformation,
		}
	}
}

// matchAllProperties checks all fields in the option set and returns true if any match.
func (m *Matcher) matchAllProperties(book *ComicBook) bool {
	for _, v := range allPropertiesFields(book, m.AllPropertiesOption) {
		if m.matchString(v) {
			return true
		}
	}
	return false
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
func evaluateMatcher(xmlMatcher *ComicBookMatcher, book *ComicBook, ctx *evalCtx) bool {
	// Check if this is a group matcher
	if strings.Contains(xmlMatcher.Type, "GroupMatcher") {
		groupMode := xmlMatcher.MatcherMode
		if groupMode == "" {
			groupMode = "And"
		}

		if len(xmlMatcher.Matchers) == 0 {
			return false
		}

		var result bool
		if groupMode == "Or" {
			result = false
			for i := range xmlMatcher.Matchers {
				if evaluateMatcher(&xmlMatcher.Matchers[i], book, ctx) {
					result = true
					break
				}
			}
		} else {
			result = true
			for i := range xmlMatcher.Matchers {
				if !evaluateMatcher(&xmlMatcher.Matchers[i], book, ctx) {
					result = false
					break
				}
			}
		}

		if xmlMatcher.Not {
			return !result
		}
		return result
	}

	// Duplicate matcher: uses pre-computed cross-book duplicate set
	if xmlMatcher.Type == "ComicBookDuplicateMatcher" {
		op := xmlMatcher.MatchOperator
		var result bool
		if op == "" || op == "0" {
			result = ctx.duplicates[book.ID]
		} else {
			result = true
		}
		if xmlMatcher.Not {
			return !result
		}
		return result
	}

	// ComicVine enrichment matchers
	if isCVMatcherType(xmlMatcher.Type) {
		return evaluateCVMatcher(xmlMatcher, book, ctx)
	}

	// Series aggregate matchers need pre-computed stats
	if isSeriesMatcherType(xmlMatcher.Type) {
		return evaluateSeriesMatcher(xmlMatcher, book, ctx.stats)
	}

	// Regular value matcher
	matcher, err := NewMatcherFromXML(xmlMatcher)
	if err != nil {
		return false
	}

	return matcher.Match(book)
}

// evaluateCVMatcher evaluates a ComicVine enrichment matcher against a book.
func evaluateCVMatcher(xmlMatcher *ComicBookMatcher, book *ComicBook, ctx *evalCtx) bool {
	cv := ctx.cvData[book.ID]

	mt := cvMatcherTypes[xmlMatcher.Type]
	m, err := NewMatcherFromXML(xmlMatcher)
	if err != nil {
		return false
	}

	// If no CV data for this book, treat as "Unknown"
	if cv == nil {
		switch mt {
		case MatcherTypeCVSeriesComplete:
			result := m.matchYesNo("Unknown")
			if m.Not {
				return !result
			}
			return result
		default:
			return m.Not // no data → no match; Not inverts
		}
	}

	var result bool
	switch mt {
	case MatcherTypeCVSeriesComplete:
		result = m.matchYesNo(cv.IsComplete)
	case MatcherTypeCVMissingCount:
		result = m.matchNumeric(strconv.Itoa(cv.MissingCount))
	case MatcherTypeCVPercentOwned:
		result = m.matchNumeric(strconv.Itoa(cv.PercentOwned))
	default:
		return false
	}
	if m.Not {
		return !result
	}
	return result
}

// evaluateSeriesMatcher evaluates a series aggregate matcher against a book using pre-computed stats.
// Returns false when stats are unavailable (no stats map or series not found).
func evaluateSeriesMatcher(xmlMatcher *ComicBookMatcher, book *ComicBook, stats map[seriesKey]*SeriesStats) bool {
	if stats == nil {
		return false
	}
	k := seriesKeyFor(book)
	s, ok := stats[k]
	if !ok {
		return false
	}

	m, err := NewMatcherFromXML(xmlMatcher)
	if err != nil {
		return false
	}

	var result bool
	switch m.Type {
	case MatcherTypeSeriesAllComplete:
		result = m.matchYesNo(s.AllComplete)
	case MatcherTypeSeriesEndOfGap:
		n, numOK := parseIssueNumber(book.Number)
		if !numOK {
			result = m.matchYesNo("Unknown")
		} else if s.GapEnds[n] {
			result = m.matchYesNo("Yes")
		} else {
			result = m.matchYesNo("No")
		}
	case MatcherTypeSeriesStartOfGap:
		n, numOK := parseIssueNumber(book.Number)
		if !numOK {
			result = m.matchYesNo("Unknown")
		} else if s.GapStarts[n] {
			result = m.matchYesNo("Yes")
		} else {
			result = m.matchYesNo("No")
		}
	case MatcherTypeSeriesAverageCommunityRating:
		result = m.matchNumeric(fmt.Sprintf("%g", s.AverageCommunityRating))
	case MatcherTypeSeriesAverageRating:
		result = m.matchNumeric(fmt.Sprintf("%g", s.AverageRating))
	case MatcherTypeSeriesCount:
		result = m.matchNumeric(fmt.Sprintf("%d", s.Count))
	case MatcherTypeSeriesFirstNumber:
		if s.FirstNumber < 0 {
			return m.Not // no numeric issues → treated as not matching; Not inverts
		}
		result = m.matchNumeric(fmt.Sprintf("%g", s.FirstNumber))
	case MatcherTypeSeriesLastNumber:
		if s.LastNumber < 0 {
			return m.Not
		}
		result = m.matchNumeric(fmt.Sprintf("%g", s.LastNumber))
	case MatcherTypeSeriesGaps:
		result = m.matchNumeric(fmt.Sprintf("%d", s.GapCount))
	case MatcherTypeSeriesMaxGapSize:
		result = m.matchNumeric(fmt.Sprintf("%g", s.MaxGapSize))
	case MatcherTypeSeriesMaxCount:
		if s.MaxCount == 0 {
			return m.Not
		}
		result = m.matchNumeric(fmt.Sprintf("%d", s.MaxCount))
	case MatcherTypeSeriesMinCount:
		if s.MinCount == 0 {
			return m.Not
		}
		result = m.matchNumeric(fmt.Sprintf("%d", s.MinCount))
	case MatcherTypeSeriesMaxYear:
		result = m.matchNumeric(fmt.Sprintf("%d", s.MaxYear))
	case MatcherTypeSeriesMinYear:
		result = m.matchNumeric(fmt.Sprintf("%d", s.MinYear))
	case MatcherTypeSeriesPageCount:
		result = m.matchNumeric(fmt.Sprintf("%d", s.PageCount))
	case MatcherTypeSeriesPagesRead:
		result = m.matchNumeric(fmt.Sprintf("%d", s.PageReadCount))
	case MatcherTypeSeriesPercentRead:
		result = m.matchNumeric(fmt.Sprintf("%d", s.ReadPercentage))
	case MatcherTypeSeriesRunningTimeYears:
		result = m.matchNumeric(fmt.Sprintf("%d", s.RunningTimeYears))
	case MatcherTypeSeriesLastAddedTime:
		result = m.matchDate(s.LastAddedTime.Format("2006-01-02T15:04:05"))
	case MatcherTypeSeriesLastOpenedTime:
		result = m.matchDate(s.LastOpenedTime.Format("2006-01-02T15:04:05"))
	case MatcherTypeSeriesLastPublishedTime:
		result = m.matchDate(s.LastPublishedTime.Format("2006-01-02T15:04:05"))
	case MatcherTypeSeriesLastReleasedTime:
		result = m.matchDate(s.LastReleasedTime.Format("2006-01-02T15:04:05"))
	default:
		return false
	}
	if m.Not {
		return !result
	}
	return result
}

// hasSeriesMatchers returns true if any matcher in the list (including nested groups) is a series aggregate matcher.
func hasSeriesMatchers(matchers []ComicBookMatcher) bool {
	for i := range matchers {
		if isSeriesMatcherType(matchers[i].Type) {
			return true
		}
		if len(matchers[i].Matchers) > 0 && hasSeriesMatchers(matchers[i].Matchers) {
			return true
		}
	}
	return false
}

// CVCompleteness holds pre-computed ComicVine series completeness data for a book.
// Keyed by book ID in the evalCtx.cvData map.
type CVCompleteness struct {
	TotalIssues  int
	OwnedIssues  int
	MissingCount int
	PercentOwned int
	IsComplete   string // "Yes", "No", "Unknown"
}

// evalCtx holds pre-computed cross-book data used during matcher evaluation.
type evalCtx struct {
	stats      map[seriesKey]*SeriesStats
	duplicates map[string]bool
	cvData     map[string]*CVCompleteness // book ID → completeness
}

// cvMatcherTypes maps XML type names to library MatcherType constants for CV matchers.
var cvMatcherTypes = map[string]MatcherType{
	"ComicServerCVSeriesCompleteMatcher": MatcherTypeCVSeriesComplete,
	"ComicServerCVMissingCountMatcher":   MatcherTypeCVMissingCount,
	"ComicServerCVPercentOwnedMatcher":   MatcherTypeCVPercentOwned,
}

func isCVMatcherType(xmlType string) bool {
	_, ok := cvMatcherTypes[xmlType]
	return ok
}

func hasCVMatcher(matchers []ComicBookMatcher) bool {
	for i := range matchers {
		if isCVMatcherType(matchers[i].Type) {
			return true
		}
		if len(matchers[i].Matchers) > 0 && hasCVMatcher(matchers[i].Matchers) {
			return true
		}
	}
	return false
}

// commonSeparators mirrors ComicRackCE's StringUtility.CommonSeparators.
var commonSeparators = func() map[rune]bool {
	m := make(map[rune]bool)
	for _, r := range " \t\n\r-~,.:;/\\'`´" {
		m[r] = true
	}
	return m
}()

// defaultArticles mirrors ComicRackCE's default StringUtility.Articles.
var defaultArticles = map[string]bool{
	"the": true, "der": true, "die": true, "das": true,
	"le": true, "la": true, "les": true, "l'": true,
}

// compressedSeriesName strips common separators and articles from a series name and
// uppercases the result, mirroring ComicRackCE's GroupInfo.CompressedName for use
// in duplicate detection.
func compressedSeriesName(text string) string {
	words := strings.FieldsFunc(text, func(r rune) bool { return commonSeparators[r] })
	var b strings.Builder
	for _, w := range words {
		if w == "" {
			continue
		}
		if !defaultArticles[strings.ToLower(w)] {
			b.WriteString(w)
		}
	}
	return strings.ToUpper(b.String())
}

// dupMetaKey is the grouping key for metadata-based duplicate detection.
// Mirrors ComicBookDuplicateComparer.EqualityComparer from ComicRackCE.
type dupMetaKey struct {
	series string // compressedSeriesName(Series), uppercased
	format string
	volume int
	number string
	lang   string
	year   int // 0 = unset (XML omitempty means missing → 0 in Go)
	month  int
	day    int
	bw     string
}

// buildDuplicateSet pre-processes all candidate books and returns the set of book IDs
// that are duplicates (by metadata or file path), matching ComicBookDuplicateMatcher logic.
func buildDuplicateSet(books []*ComicBook) map[string]bool {
	metaGroups := make(map[dupMetaKey][]string)
	pathGroups := make(map[string][]string)

	for _, book := range books {
		key := dupMetaKey{
			series: compressedSeriesName(book.Series),
			format: book.Format,
			volume: book.Volume,
			number: book.Number,
			lang:   book.LanguageISO,
			year:   book.Year,
			month:  book.Month,
			day:    book.Day,
			bw:     book.BlackAndWhite,
		}
		metaGroups[key] = append(metaGroups[key], book.ID)

		if book.FilePath != "" {
			normPath := strings.ToLower(strings.ReplaceAll(book.FilePath, "\\", "/"))
			pathGroups[normPath] = append(pathGroups[normPath], book.ID)
		}
	}

	duplicates := make(map[string]bool)
	for _, ids := range metaGroups {
		if len(ids) > 1 {
			for _, id := range ids {
				duplicates[id] = true
			}
		}
	}
	for _, ids := range pathGroups {
		if len(ids) > 1 {
			for _, id := range ids {
				duplicates[id] = true
			}
		}
	}
	return duplicates
}

// hasDuplicateMatcher reports whether any matcher in the list (including nested groups) is a
// duplicate matcher.
func hasDuplicateMatcher(matchers []ComicBookMatcher) bool {
	for i := range matchers {
		if matchers[i].Type == "ComicBookDuplicateMatcher" {
			return true
		}
		if len(matchers[i].Matchers) > 0 && hasDuplicateMatcher(matchers[i].Matchers) {
			return true
		}
	}
	return false
}

// matchBooks evaluates a list's matchers against a slice of candidate books.
// cvData is optional ComicVine enrichment data keyed by book ID (nil if unavailable).
func matchBooks(list *ComicListItem, candidates []*ComicBook, cvData map[string]*CVCompleteness) []*ComicBook {
	matcherMode := list.MatcherMode
	if matcherMode == "" {
		matcherMode = "And"
	}

	// Build evaluation context with pre-computed cross-book data.
	ctx := &evalCtx{cvData: cvData}
	if hasSeriesMatchers(list.Matchers) {
		ctx.stats = buildSeriesStats(candidates)
	}
	if hasDuplicateMatcher(list.Matchers) {
		ctx.duplicates = buildDuplicateSet(candidates)
	}

	var matched []*ComicBook
	for _, book := range candidates {
		var ok bool
		if matcherMode == "Or" {
			for j := range list.Matchers {
				if evaluateMatcher(&list.Matchers[j], book, ctx) {
					ok = true
					break
				}
			}
		} else {
			ok = true
			for j := range list.Matchers {
				if !evaluateMatcher(&list.Matchers[j], book, ctx) {
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

	return matchBooks(list, candidates, l.cvData), nil
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
