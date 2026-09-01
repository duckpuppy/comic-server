package datamanager

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// fieldRefPattern matches a {FieldName} reference inside an action value -
// confirmed real usage: action field="SeriesGroup" modifier="SetValue"
// value="{AlternateSeries}" (see comic-server-764's design notes).
var fieldRefPattern = regexp.MustCompile(`\{([A-Za-z0-9 ]+)\}`)

// SubstituteFieldRefs replaces every {FieldName} reference in value with
// that field's current value on book, for both built-in fields (via
// GetFieldString) and custom values (via CustomValuesStore). A reference
// to an unknown field is left as-is rather than silently dropped, so a
// rule-authoring mistake is visible in the result instead of vanishing.
func SubstituteFieldRefs(value string, book *library.ComicBook) string {
	return fieldRefPattern.ReplaceAllStringFunc(value, func(match string) string {
		name := strings.TrimSpace(match[1 : len(match)-1])
		if s, ok := GetFieldString(book, name); ok {
			return s
		}
		if IsCustomField(name) {
			if s, ok := getCustomValue(book, name); ok {
				return s
			}
		}
		return match
	})
}

func getCustomValue(book *library.ComicBook, key string) (string, bool) {
	for pair := range strings.SplitSeq(book.CustomValuesStore, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, found := strings.Cut(pair, "=")
		if found && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// ApplyAction applies one Data Manager action to book in place, returning
// whether anything actually changed. See comic-server-764's design notes
// for each modifier's confirmed real-world semantics (ground-truthed
// against the user's actual dataman.dat where real usage exists).
func ApplyAction(a Action, book *library.ComicBook) (bool, error) {
	value := SubstituteFieldRefs(a.Value, book)

	// RegExVarReplace/RegExVarAppend write into OTHER fields (named
	// capture groups), not just a.Field - handled separately since every
	// other modifier only ever touches the one field it targets.
	if mod := strings.ToLower(a.Modifier); mod == "regexvarreplace" || mod == "regexvarappend" {
		return applyRegExVarReplace(a, book, value, mod == "regexvarappend")
	}

	if IsCustomField(a.Field) {
		return applyCustomFieldAction(a, book, value)
	}

	def, ok := builtinFields[a.Field]
	if !ok {
		return false, fmt.Errorf("unknown field %q", a.Field)
	}
	if !def.Writable {
		return false, fmt.Errorf("field %q is read-only, cannot be an action target", a.Field)
	}

	current, _ := GetFieldString(book, a.Field)
	multiValue := def.Kind == KindMultiValue

	newValue, changed, err := computeActionValue(a.Modifier, current, value, multiValue)
	if err != nil {
		return false, fmt.Errorf("field %q: %w", a.Field, err)
	}
	if !changed {
		return false, nil
	}
	if err := SetFieldString(book, a.Field, newValue); err != nil {
		return false, err
	}
	return true, nil
}

// applyRegExVarReplace matches pattern against a.Field's current value,
// then writes each of the regex's NAMED capture groups into the
// same-named field on the book (built-in or custom) - the real
// mechanism behind Data Manager's "strip trailing Annual into TempAnnual,
// re-append after reordering" two-pass trick (see comic-server-764's
// design notes). appendMode implements RegExVarAppend (append to the
// target field's existing value instead of overwriting it).
//
// Not verified against real usage - the user's actual dataman.dat never
// exercises this modifier, so this is built from the plugin's documented
// behavior only. Flag any observed mismatch if this is ever exercised
// against a real rule using it.
func applyRegExVarReplace(a Action, book *library.ComicBook, pattern string, appendMode bool) (bool, error) {
	current, ok := readAnyField(book, a.Field)
	if !ok {
		return false, fmt.Errorf("unknown field %q", a.Field)
	}

	rx, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	match := rx.FindStringSubmatch(current)
	if match == nil {
		return false, nil
	}

	changed := false
	for i, name := range rx.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		groupVal := match[i]
		existing, _ := readAnyField(book, name)
		newVal := groupVal
		if appendMode {
			newVal = existing + groupVal
		}
		if newVal == existing {
			continue
		}
		if err := writeAnyField(book, name, newVal); err != nil {
			return changed, fmt.Errorf("writing captured group %q: %w", name, err)
		}
		changed = true
	}
	return changed, nil
}

// readAnyField reads field as either a built-in field or a custom value.
func readAnyField(book *library.ComicBook, field string) (string, bool) {
	if IsCustomField(field) {
		return getCustomValue(book, field)
	}
	return GetFieldString(book, field)
}

// writeAnyField writes value to field as either a built-in field or a
// custom value.
func writeAnyField(book *library.ComicBook, field, value string) error {
	if IsCustomField(field) {
		book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, field, value)
		return nil
	}
	return SetFieldString(book, field, value)
}

func applyCustomFieldAction(a Action, book *library.ComicBook, value string) (bool, error) {
	current, _ := getCustomValue(book, a.Field)

	newValue, changed, err := computeActionValue(a.Modifier, current, value, false)
	if err != nil {
		return false, fmt.Errorf("custom field %q: %w", a.Field, err)
	}
	if !changed {
		return false, nil
	}
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, a.Field, newValue)
	return true, nil
}

// computeActionValue implements the actual per-modifier semantics, field-
// kind-agnostic except where SetValue/Add/Remove genuinely differ between
// a multi-value (comma-list) field and a plain string. Returns the new
// value and whether it differs from current.
func computeActionValue(modifier, current, value string, multiValue bool) (string, bool, error) {
	switch strings.ToLower(modifier) {
	case "setvalue":
		return value, current != value, nil

	case "add":
		if multiValue {
			return addListItem(current, value), !hasListItem(current, value), nil
		}
		// String fields: plain concatenation, no separator inserted -
		// matches the real tool's own behavior exactly (confirmed from
		// the user's real rules, which compensate by including their
		// own leading space in the action value, e.g. Add " Annual").
		return current + value, value != "", nil

	case "remove":
		if multiValue {
			newVal := removeListItem(current, value)
			return newVal, newVal != current, nil
		}
		newVal := strings.ReplaceAll(current, value, "")
		return newVal, newVal != current, nil

	case "removeleading":
		newVal := strings.TrimPrefix(current, value)
		return newVal, newVal != current, nil

	case "replace":
		parts := splitParams(value)
		if len(parts) != 2 {
			return "", false, fmt.Errorf("Replace value %q must have exactly 2 \"||\"-joined parts (old||new)", value)
		}
		newVal := strings.ReplaceAll(current, parts[0], parts[1])
		return newVal, newVal != current, nil

	case "regexreplace":
		parts := splitParams(value)
		if len(parts) != 2 {
			return "", false, fmt.Errorf("RegexReplace value %q must have exactly 2 \"||\"-joined parts (pattern||replacement)", value)
		}
		rx, err := regexp.Compile(parts[0])
		if err != nil {
			return "", false, fmt.Errorf("invalid regex %q: %w", parts[0], err)
		}
		newVal := rx.ReplaceAllString(current, parts[1])
		return newVal, newVal != current, nil

	case "calc":
		// The original plugin runs Python eval() on the value after
		// reference substitution - comic-server has no safe Go
		// equivalent and this was flagged as an open design question in
		// comic-server-764 rather than assumed away. Not in the user's
		// real dataman.dat (zero occurrences), so this is a deliberate
		// "not supported yet" rather than a silent no-op.
		return "", false, fmt.Errorf("Calc is not supported (no safe expression evaluator)")

	default:
		return "", false, fmt.Errorf("unsupported action modifier %q", modifier)
	}
}

// Apply evaluates rs's rules against book and, if they match, applies
// every one of rs's actions in order, returning whether anything changed.
// Actions within a matching ruleset always all apply together - there's
// no per-action conditional in Data Manager's model, only per-ruleset.
func (rs Ruleset) Apply(book *library.ComicBook) (bool, error) {
	matched, err := rs.Matches(book)
	if err != nil {
		return false, fmt.Errorf("evaluating conditions: %w", err)
	}
	if !matched {
		return false, nil
	}

	changed := false
	for _, a := range rs.Actions {
		did, err := ApplyAction(a, book)
		if err != nil {
			return changed, fmt.Errorf("action %+v: %w", a, err)
		}
		if did {
			changed = true
		}
	}
	return changed, nil
}

func hasListItem(list, item string) bool {
	for _, v := range strings.Split(list, ",") {
		if strings.TrimSpace(v) == item {
			return true
		}
	}
	return false
}

func addListItem(list, item string) string {
	if hasListItem(list, item) {
		return list
	}
	items := splitListItems(list)
	items = append(items, item)
	return strings.Join(items, ", ")
}

func removeListItem(list, item string) string {
	items := splitListItems(list)
	kept := items[:0]
	for _, v := range items {
		if v != item {
			kept = append(kept, v)
		}
	}
	return strings.Join(kept, ", ")
}

func splitListItems(list string) []string {
	var items []string
	for _, v := range strings.Split(list, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			items = append(items, v)
		}
	}
	return items
}
