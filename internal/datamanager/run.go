package datamanager

import (
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// FieldChange is one field's before/after value from a rule run, for a
// preview UI to render. Custom is true for a CustomValuesStore entry
// (Concept, "Data Manager processed", etc) rather than a built-in field.
type FieldChange struct {
	Field  string
	Custom bool
	Old    string
	New    string
}

// ApplyAll runs every ruleset in rulesets against book IN ORDER (the same
// depth-first document order the real plugin evaluates in - a later
// ruleset's action can overwrite an earlier one's on the same field, see
// comic-server-764's design notes), then diffs every writable built-in
// field plus every CustomValuesStore key against a snapshot taken before
// the run. This catches a change regardless of which modifier produced
// it, including RegExVarReplace/Append writes into fields not named by
// any rs.Actions entry.
//
// Callers doing a preview (not persisting) must pass a copy of book, not
// a pointer shared with a cached library snapshot - ApplyAll mutates book
// in place.
func ApplyAll(book *library.ComicBook, rulesets []Ruleset) ([]FieldChange, error) {
	before := snapshotFields(book)
	beforeCustom := parseCustomValues(book.CustomValuesStore)

	var firstErr error
	for _, rs := range rulesets {
		if _, err := rs.Apply(book); err != nil {
			// One bad ruleset shouldn't stop the rest from running - the
			// real plugin has no concept of aborting a whole pass over one
			// ruleset's error, and later rulesets are independent. Report
			// the first error to the caller after the full run completes.
			if firstErr == nil {
				firstErr = fmt.Errorf("ruleset %q: %w", rs.Name, err)
			}
		}
	}

	after := snapshotFields(book)
	afterCustom := parseCustomValues(book.CustomValuesStore)

	var changes []FieldChange
	for field, oldVal := range before {
		if newVal := after[field]; newVal != oldVal {
			changes = append(changes, FieldChange{Field: field, Old: oldVal, New: newVal})
		}
	}
	for key, oldVal := range beforeCustom {
		newVal, stillPresent := afterCustom[key]
		if !stillPresent {
			changes = append(changes, FieldChange{Field: key, Custom: true, Old: oldVal, New: ""})
		} else if newVal != oldVal {
			changes = append(changes, FieldChange{Field: key, Custom: true, Old: oldVal, New: newVal})
		}
	}
	for key, newVal := range afterCustom {
		if _, existed := beforeCustom[key]; !existed {
			changes = append(changes, FieldChange{Field: key, Custom: true, Old: "", New: newVal})
		}
	}

	return changes, firstErr
}

// snapshotFields reads every writable built-in field off book, for
// before/after diffing - read-only fields (FilePath, FileFormat, ...)
// can never change from a rule run, so they're excluded.
func snapshotFields(book *library.ComicBook) map[string]string {
	out := make(map[string]string, len(builtinFields))
	for field, def := range builtinFields {
		if !def.Writable {
			continue
		}
		if s, ok := GetFieldString(book, field); ok {
			out[field] = s
		}
	}
	return out
}

func parseCustomValues(store string) map[string]string {
	out := map[string]string{}
	for pair := range strings.SplitSeq(store, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, found := strings.Cut(pair, "=")
		if found {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}
