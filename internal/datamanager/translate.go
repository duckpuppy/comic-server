package datamanager

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// errBoolGetterField signals that a rule's field has no MatcherType
// equivalent and must be evaluated directly via its FieldDef.BoolGetter
// (see evaluate.go) rather than through a ComicBookMatcher tree.
var errBoolGetterField = errors.New("field has no matcher equivalent, evaluate via BoolGetter")

// listDelimiter is dataman.ini's ListDelimiter setting - how a multi-
// parameter rule/action value (IsAnyOf's list, Range's two bounds, ...)
// packs multiple values into one XML attribute. Distinct from ComicRack's
// own multi-value field storage, which uses ",".
const listDelimiter = "||"

// splitParams splits a "||"-joined multi-parameter value.
func splitParams(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, listDelimiter)
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// dmDateToRFC3339 converts a dataman.dat date value ("2024/10/01",
// confirmed from the user's real file) to the RFC3339 format
// internal/library's date matchers expect.
func dmDateToRFC3339(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	t, err := time.Parse("2006/01/02", v)
	if err != nil {
		// Fall back to RFC3339 in case a rule was authored/exported in
		// that format already.
		if _, err2 := time.Parse(time.RFC3339, v); err2 == nil {
			return v, nil
		}
		return "", fmt.Errorf("unrecognized date value %q: %w", v, err)
	}
	return t.Format(time.RFC3339), nil
}

// xsiType returns the ComicRack xsi:type string for a built-in
// MatcherType (e.g. "Series" -> "ComicBookSeriesMatcher"), following the
// same "ComicBook"+name+"Matcher" convention every existing matcher type
// in internal/library already uses.
func xsiType(mt library.MatcherType) string {
	return "ComicBook" + string(mt) + "Matcher"
}

// valueMatcher builds a single (non-group) ComicBookMatcher: xsi:type for
// fieldType, numeric operator code op, and value(s).
func valueMatcher(mt library.MatcherType, op library.MatchOperator, not bool, value, value2 string) library.ComicBookMatcher {
	return library.ComicBookMatcher{
		Type:          xsiType(mt),
		Not:           not,
		MatchOperator: fmt.Sprintf("%d", op),
		MatchValue:    value,
		MatchValue2:   value2,
	}
}

// groupMatcher builds an AND/OR group of child matchers.
func groupMatcher(mode string, not bool, children ...library.ComicBookMatcher) library.ComicBookMatcher {
	return library.ComicBookMatcher{
		Type:        "ComicBookGroupMatcher",
		Not:         not,
		MatcherMode: mode,
		Matchers:    children,
	}
}

// TranslateRule converts one Data Manager rule into a comic-server
// ComicBookMatcher tree, reusing the existing smart-list matcher engine's
// operator vocabulary directly where one exists, and expanding modifiers
// with no single-operator equivalent (IsAnyOf, StartsWithAnyOf,
// GreaterEq, LesserEq) into an OR-group of existing operators - see
// comic-server-764's design notes for why this is preferred over adding
// new operator codes to the shared engine.
func TranslateRule(r Rule) (library.ComicBookMatcher, error) {
	if IsCustomField(r.Field) {
		return translateCustomFieldRule(r)
	}

	def, ok := builtinFields[r.Field]
	if !ok {
		return library.ComicBookMatcher{}, fmt.Errorf("unknown field %q", r.Field)
	}
	if def.MatcherType == "" {
		return library.ComicBookMatcher{}, fmt.Errorf("field %q has no condition support yet", r.Field)
	}

	switch def.Kind {
	case KindString:
		return translateStringRule(def.MatcherType, r)
	case KindNumeric, KindPseudoNumeric:
		return translateNumericRule(def.MatcherType, r)
	case KindDate:
		return translateDateRule(def.MatcherType, r)
	case KindBool, KindYesNo, KindManga:
		return translateBoolRule(def, r)
	case KindLanguage:
		return translateLanguageRule(def.MatcherType, r)
	case KindMultiValue:
		// Multi-value fields (Tags, Genre, Writer, ...) are stored as a
		// comma-separated string on the book and read the same way any
		// other string field is for condition purposes - ComicRack's own
		// matchers treat them identically to KindString here.
		return translateStringRule(def.MatcherType, r)
	default:
		return library.ComicBookMatcher{}, fmt.Errorf("field %q: unhandled kind", r.Field)
	}
}

// customValueOperator maps a Data Manager string modifier to a
// comic-server operator code, for use against a custom value via
// internal/library's CustomValues matcher - extended (see
// comic-server-764.3 / internal/library/smartlist.go) to support the same
// Contains/StartsWith/Regex/ContainsAny/ContainsAll operators any other
// string field gets, not just exact-equals.
func customValueOperator(modifier string) (op library.MatchOperator, not bool, ok bool) {
	mod := strings.ToLower(modifier)
	not = strings.HasPrefix(mod, "not")
	switch strings.TrimPrefix(mod, "not") {
	case "is":
		return library.OperatorEquals, not, true
	case "contains":
		return library.OperatorContains, not, true
	case "startswith":
		return library.OperatorStartsWith, not, true
	case "regex":
		return library.OperatorRegex, not, true
	case "containsanyof":
		return library.OperatorContainsAny, not, true
	case "containsallof":
		return library.OperatorContainsAll, not, true
	default:
		return 0, false, false
	}
}

func translateCustomFieldRule(r Rule) (library.ComicBookMatcher, error) {
	// "Not" alone (bare, not a NotX compound) is Data Manager's own
	// negated-equals shorthand for custom fields specifically - fold it
	// into customValueOperator's Is+not path rather than adding a
	// separate case.
	modifier := r.Modifier
	if strings.EqualFold(modifier, "not") {
		modifier = "NotIs"
	}

	op, not, ok := customValueOperator(modifier)
	if !ok {
		return library.ComicBookMatcher{}, fmt.Errorf("custom field %q: unsupported modifier %q", r.Field, r.Modifier)
	}

	value := r.Value
	if op == library.OperatorContainsAny || op == library.OperatorContainsAll {
		value = strings.Join(splitParams(r.Value), ",")
	}

	return library.ComicBookMatcher{
		Type:          "ComicBookCustomValuesMatcher",
		Not:           not,
		MatchOperator: fmt.Sprintf("%d", op),
		MatchValue:    r.Field,
		MatchValue2:   value,
	}, nil
}

// normalizeBareNot lowercases modifier and folds Data Manager's bare "Not"
// (negated-equals shorthand, confirmed real usage on built-in string
// fields like "SeriesGroup Not Ultimate Marvel" - see comic-server-764.6's
// design notes) into "notis", so the shared not/base HasPrefix/TrimPrefix
// split below produces base="is" instead of an empty base that matches no
// case and errors as unsupported. translateCustomFieldRule and
// translateBoolRule handle this same shorthand their own way already;
// this covers the remaining kinds (string/numeric/date/language) that
// share this modifier-parsing shape.
func normalizeBareNot(modifier string) string {
	mod := strings.ToLower(modifier)
	if mod == "not" {
		return "notis"
	}
	return mod
}

func translateStringRule(mt library.MatcherType, r Rule) (library.ComicBookMatcher, error) {
	mod := normalizeBareNot(r.Modifier)
	not := strings.HasPrefix(mod, "not")
	base := strings.TrimPrefix(mod, "not")

	switch base {
	case "is":
		return valueMatcher(mt, library.OperatorEquals, not, r.Value, ""), nil
	case "contains":
		return valueMatcher(mt, library.OperatorContains, not, r.Value, ""), nil
	case "startswith":
		return valueMatcher(mt, library.OperatorStartsWith, not, r.Value, ""), nil
	case "regex":
		return valueMatcher(mt, library.OperatorRegex, not, r.Value, ""), nil
	case "containsanyof":
		return valueMatcher(mt, library.OperatorContainsAny, not, strings.Join(splitParams(r.Value), ","), ""), nil
	case "containsallof":
		return valueMatcher(mt, library.OperatorContainsAll, not, strings.Join(splitParams(r.Value), ","), ""), nil
	case "isanyof":
		return orGroupOfEquals(mt, not, splitParams(r.Value)), nil
	case "startswithanyof":
		return orGroupOfStartsWith(mt, not, splitParams(r.Value)), nil
	default:
		return library.ComicBookMatcher{}, fmt.Errorf("field %q: unsupported string modifier %q", r.Field, r.Modifier)
	}
}

func orGroupOfEquals(mt library.MatcherType, not bool, values []string) library.ComicBookMatcher {
	children := make([]library.ComicBookMatcher, len(values))
	for i, v := range values {
		children[i] = valueMatcher(mt, library.OperatorEquals, false, v, "")
	}
	return groupMatcher("Or", not, children...)
}

func orGroupOfStartsWith(mt library.MatcherType, not bool, values []string) library.ComicBookMatcher {
	children := make([]library.ComicBookMatcher, len(values))
	for i, v := range values {
		children[i] = valueMatcher(mt, library.OperatorStartsWith, false, v, "")
	}
	return groupMatcher("Or", not, children...)
}

func translateNumericRule(mt library.MatcherType, r Rule) (library.ComicBookMatcher, error) {
	mod := normalizeBareNot(r.Modifier)
	not := strings.HasPrefix(mod, "not")
	base := strings.TrimPrefix(mod, "not")

	switch base {
	case "is":
		return valueMatcher(mt, library.OperatorEquals, not, r.Value, ""), nil
	case "greater":
		return valueMatcher(mt, library.OperatorGreater, not, r.Value, ""), nil
	case "less":
		return valueMatcher(mt, library.OperatorLesser, not, r.Value, ""), nil
	case "greatereq":
		// >= X  ==  Greater X OR Equals X - see comic-server-764's design
		// notes on why this expands rather than needing a new operator.
		return groupMatcher("Or", not,
			valueMatcher(mt, library.OperatorGreater, false, r.Value, ""),
			valueMatcher(mt, library.OperatorEquals, false, r.Value, ""),
		), nil
	case "lesseq":
		return groupMatcher("Or", not,
			valueMatcher(mt, library.OperatorLesser, false, r.Value, ""),
			valueMatcher(mt, library.OperatorEquals, false, r.Value, ""),
		), nil
	case "range":
		parts := splitParams(r.Value)
		if len(parts) != 2 {
			return library.ComicBookMatcher{}, fmt.Errorf("field %q: Range value %q must have exactly 2 parts", r.Field, r.Value)
		}
		return valueMatcher(mt, library.OperatorInRange, not, parts[0], parts[1]), nil
	case "isanyof":
		return orGroupOfEquals(mt, not, splitParams(r.Value)), nil
	default:
		return library.ComicBookMatcher{}, fmt.Errorf("field %q: unsupported numeric modifier %q", r.Field, r.Modifier)
	}
}

func translateDateRule(mt library.MatcherType, r Rule) (library.ComicBookMatcher, error) {
	mod := normalizeBareNot(r.Modifier)
	not := strings.HasPrefix(mod, "not")
	base := strings.TrimPrefix(mod, "not")

	// IsInLastDays' value is a day count, not a date - skip conversion.
	if base == "isinlastdays" {
		return valueMatcher(mt, library.OperatorIsInLastDays, not, r.Value, ""), nil
	}

	if base == "range" {
		parts := splitParams(r.Value)
		if len(parts) != 2 {
			return library.ComicBookMatcher{}, fmt.Errorf("field %q: Range value %q must have exactly 2 parts", r.Field, r.Value)
		}
		v1, err := dmDateToRFC3339(parts[0])
		if err != nil {
			return library.ComicBookMatcher{}, err
		}
		v2, err := dmDateToRFC3339(parts[1])
		if err != nil {
			return library.ComicBookMatcher{}, err
		}
		return valueMatcher(mt, library.OperatorIsInDateRange, not, v1, v2), nil
	}

	value, err := dmDateToRFC3339(r.Value)
	if err != nil {
		return library.ComicBookMatcher{}, err
	}

	switch base {
	case "is":
		return valueMatcher(mt, library.OperatorEquals, not, value, ""), nil
	case "greater":
		return valueMatcher(mt, library.OperatorIsAfter, not, value, ""), nil
	case "less":
		return valueMatcher(mt, library.OperatorIsBefore, not, value, ""), nil
	case "greatereq":
		return groupMatcher("Or", not,
			valueMatcher(mt, library.OperatorIsAfter, false, value, ""),
			valueMatcher(mt, library.OperatorEquals, false, value, ""),
		), nil
	case "lesseq":
		return groupMatcher("Or", not,
			valueMatcher(mt, library.OperatorIsBefore, false, value, ""),
			valueMatcher(mt, library.OperatorEquals, false, value, ""),
		), nil
	default:
		return library.ComicBookMatcher{}, fmt.Errorf("field %q: unsupported date modifier %q", r.Field, r.Modifier)
	}
}

func translateBoolRule(def FieldDef, r Rule) (library.ComicBookMatcher, error) {
	mod := strings.ToLower(r.Modifier)
	if mod != "is" && mod != "not" {
		return library.ComicBookMatcher{}, fmt.Errorf("field %q: unsupported bool modifier %q", r.Field, r.Modifier)
	}
	// dataman.dat encodes the target state in the value ("Yes"/"No"),
	// with Is/Not only controlling whether the check is negated - so
	// "Is No" and "Not Yes" are equivalent forms of the same condition.
	// Fold both into a single true/false target before choosing the
	// operator, matching internal/library's Yes/No/Unknown-as-operator
	// convention for these field kinds (see smartlist.go's
	// OperatorEqualsYes/No/Unknown).
	wantYes := strings.EqualFold(strings.TrimSpace(r.Value), "yes") || strings.EqualFold(strings.TrimSpace(r.Value), "true")
	if mod == "not" {
		wantYes = !wantYes
	}

	if def.Kind == KindBool && def.BoolGetter != nil {
		// No MatcherType for this field - not currently supported as a
		// rule condition through the shared matcher engine. Evaluated
		// directly in evaluate.go instead of via a ComicBookMatcher.
		return library.ComicBookMatcher{}, errBoolGetterField
	}

	op := library.OperatorEqualsYes
	if !wantYes {
		op = library.OperatorEqualsNo
	}
	return valueMatcher(def.MatcherType, op, false, "", ""), nil
}

func translateLanguageRule(mt library.MatcherType, r Rule) (library.ComicBookMatcher, error) {
	mod := normalizeBareNot(r.Modifier)
	not := strings.HasPrefix(mod, "not")
	base := strings.TrimPrefix(mod, "not")

	switch base {
	case "is":
		return valueMatcher(mt, library.OperatorEquals, not, r.Value, ""), nil
	case "isanyof":
		return orGroupOfEquals(mt, not, splitParams(r.Value)), nil
	default:
		return library.ComicBookMatcher{}, fmt.Errorf("field %q: unsupported language modifier %q", r.Field, r.Modifier)
	}
}
