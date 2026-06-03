package library

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatMatcher converts a ComicBookMatcher to human-readable string
func FormatMatcher(m ComicBookMatcher) string {
	// Check if this is a group matcher
	if strings.Contains(m.Type, "GroupMatcher") {
		mode := "ALL"
		if m.MatcherMode == "Or" {
			mode = "ANY"
		}

		negation := ""
		if m.Not {
			negation = "NOT "
		}

		// Format as: "Match ALL of:" or "Match ANY of:"
		return fmt.Sprintf("%sMatch %s of %d conditions", negation, mode, len(m.Matchers))
	}

	// Parse operator as integer
	opInt, err := strconv.Atoi(m.MatchOperator)
	if err != nil {
		// If not a number, use as-is
		return fmt.Sprintf("%s %s '%s'", m.Type, m.MatchOperator, m.MatchValue)
	}

	operator := MatchOperator(opInt)
	var operatorText string

	// Determine operator text based on the type of field and operator
	matcherType := MatcherType(m.Type)

	// Check if this is a date/time field
	isDateField := matcherType == MatcherTypeYear ||
		matcherType == MatcherTypeMonth ||
		matcherType == MatcherTypeAddedTime ||
		matcherType == MatcherTypeOpenedTime

	// Check if this is a numeric field
	isNumericField := matcherType == MatcherTypePageCount ||
		matcherType == MatcherTypeVolume ||
		matcherType == MatcherTypeNumber ||
		matcherType == MatcherTypeRating

	// Map operator to text based on field type
	switch operator {
	case OperatorEquals:
		operatorText = "equals"
	case OperatorContains:
		if isDateField {
			operatorText = "is after"
		} else if isNumericField {
			operatorText = "is greater than"
		} else {
			operatorText = "contains"
		}
	case OperatorContainsAny:
		if isDateField {
			operatorText = "is before"
		} else if isNumericField {
			operatorText = "is less than"
		} else {
			operatorText = "contains any of"
		}
	case OperatorContainsAll:
		if isDateField {
			operatorText = "is in last days"
		} else if isNumericField {
			operatorText = "is in range"
		} else {
			operatorText = "contains all of"
		}
	case OperatorStartsWith:
		if isDateField {
			operatorText = "is in date range"
		} else {
			operatorText = "starts with"
		}
	case OperatorEndsWith:
		operatorText = "ends with"
	case OperatorRegex:
		operatorText = "matches regex"
	default:
		operatorText = fmt.Sprintf("op(%d)", operator)
	}

	// Format the value with or without quotes depending on field type
	if isNumericField || isDateField {
		return fmt.Sprintf("%s %s %s", m.Type, operatorText, m.MatchValue)
	}
	return fmt.Sprintf("%s %s '%s'", m.Type, operatorText, m.MatchValue)
}

// FormatMatcherMode converts matcher mode to human-readable string
func FormatMatcherMode(mode string) string {
	switch mode {
	case "And":
		return "Match ALL (AND)"
	case "Or":
		return "Match ANY (OR)"
	default:
		return mode
	}
}

// MatcherInfo represents a matcher in a structured format for API responses
type MatcherInfo struct {
	Field    string        `json:"field"`
	Operator string        `json:"operator"`
	Value    string        `json:"value"`
	Value2   string        `json:"value2,omitempty"`
	Children []MatcherInfo `json:"children,omitempty"`
}

// GetMatcherInfo converts a ComicBookMatcher to structured MatcherInfo
func GetMatcherInfo(m ComicBookMatcher) MatcherInfo {
	// Check if this is a group matcher
	if strings.Contains(m.Type, "GroupMatcher") {
		mode := "Match ALL"
		if m.MatcherMode == "Or" {
			mode = "Match ANY"
		}

		negation := ""
		if m.Not {
			negation = "NOT "
		}

		children := make([]MatcherInfo, len(m.Matchers))
		for i, child := range m.Matchers {
			children[i] = GetMatcherInfo(child)
		}

		return MatcherInfo{
			Field:    "Group",
			Operator: negation + mode,
			Value:    fmt.Sprintf("%d conditions", len(m.Matchers)),
			Children: children,
		}
	}

	// Extract field name from matcher type
	// e.g., "ComicBookPublisherMatcher" -> "Publisher"
	fieldName := formatFieldName(m.Type)

	// Handle CustomValuesMatcher specially (MatchValue is key name, MatchValue2 is expected value)
	if strings.Contains(m.Type, "CustomValuesMatcher") {
		fieldName = "Custom Value: " + m.MatchValue
		negation := ""
		if m.Not {
			negation = "NOT "
		}
		if m.MatchValue2 == "" {
			// Key-existence check
			return MatcherInfo{
				Field:    fieldName,
				Operator: negation + "exists",
			}
		}
		return MatcherInfo{
			Field:    fieldName,
			Operator: negation + "equals",
			Value:    m.MatchValue2,
		}
	}

	// Parse operator as integer, default to Equals if not specified
	operator := OperatorEquals // Default
	if m.MatchOperator != "" {
		opInt, err := strconv.Atoi(m.MatchOperator)
		if err != nil {
			// If not a number, use as-is as operator text
			return MatcherInfo{
				Field:    fieldName,
				Operator: m.MatchOperator,
				Value:    m.MatchValue,
				Value2:   m.MatchValue2,
			}
		}
		operator = MatchOperator(opInt)
	}
	matcherType := MatcherType(m.Type)

	// Check field types
	isDateField := matcherType == MatcherTypeYear ||
		matcherType == MatcherTypeMonth ||
		matcherType == MatcherTypeAddedTime ||
		matcherType == MatcherTypeOpenedTime

	isNumericField := matcherType == MatcherTypePageCount ||
		matcherType == MatcherTypeVolume ||
		matcherType == MatcherTypeNumber ||
		matcherType == MatcherTypeRating

	// Map operator to text based on field type
	operatorText := getOperatorText(operator, isDateField, isNumericField)

	// Add negation prefix if needed
	if m.Not {
		operatorText = "NOT " + operatorText
	}

	// Only include Value2 for range operators where it's actually used
	value2 := ""
	if (isNumericField && operator == OperatorContainsAll) || // "is in range" needs both values
		(isDateField && operator == OperatorStartsWith) { // "is in date range" needs both values
		value2 = m.MatchValue2
	}

	return MatcherInfo{
		Field:    fieldName,
		Operator: operatorText,
		Value:    m.MatchValue,
		Value2:   value2,
	}
}

// formatFieldName formats a matcher type string for display
// e.g., "ComicBookPublisherMatcher" -> "Publisher"
func formatFieldName(matcherType string) string {
	// Remove "ComicBook" prefix and "Matcher" suffix
	name := strings.TrimPrefix(matcherType, "ComicBook")
	name = strings.TrimSuffix(name, "Matcher")

	// Handle special cases for better display names
	switch name {
	case "CustomValues":
		return "Custom Value"
	case "MainCharacterOrTeam":
		return "Main Character/Team"
	case "AddedTime":
		return "Date Added"
	case "OpenedTime":
		return "Date Opened"
	case "ReleasedTime":
		return "Release Date"
	case "BlackAndWhite":
		return "Black & White"
	case "AgeRating":
		return "Age Rating"
	case "LanguageISO":
		return "Language"
	case "PageCount":
		return "Page Count"
	case "SeriesComplete":
		return "Series Complete"
	case "StoryArc":
		return "Story Arc"
	case "SeriesGroup":
		return "Series Group"
	case "AlternateSeries":
		return "Alternate Series"
	case "AlternateNumber":
		return "Alternate Number"
	case "AlternateCount":
		return "Alternate Count"
	case "CoverArtist":
		return "Cover Artist"
	}

	return name
}

// getOperatorText returns human-readable operator text
func getOperatorText(operator MatchOperator, isDateField, isNumericField bool) string {
	switch operator {
	case OperatorEquals:
		return "equals"
	case OperatorContains:
		if isDateField {
			return "is after"
		} else if isNumericField {
			return "is greater than"
		}
		return "contains"
	case OperatorContainsAny:
		if isDateField {
			return "is before"
		} else if isNumericField {
			return "is less than"
		}
		return "contains any of"
	case OperatorContainsAll:
		if isDateField {
			return "is in last days"
		} else if isNumericField {
			return "is in range"
		}
		return "contains all of"
	case OperatorStartsWith:
		if isDateField {
			return "is in date range"
		}
		return "starts with"
	case OperatorEndsWith:
		return "ends with"
	case OperatorListContains:
		return "list contains"
	case OperatorRegex:
		return "matches regex"
	default:
		return fmt.Sprintf("op(%d)", operator)
	}
}
