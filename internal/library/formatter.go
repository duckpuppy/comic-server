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
