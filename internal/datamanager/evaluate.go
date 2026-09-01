package datamanager

import (
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// BuildConditionGroup wraps every non-BoolGetter rule in rules into one
// AND/OR group ComicBookMatcher, for evaluation via the existing
// internal/library matcher engine. Returns ok=false if rules contains
// only BoolGetter-only fields (nothing to build).
func BuildConditionGroup(rules []Rule, mode string) (m library.ComicBookMatcher, ok bool, err error) {
	var children []library.ComicBookMatcher
	for _, r := range rules {
		if isBoolGetterField(r.Field) {
			continue
		}
		child, err := TranslateRule(r)
		if err != nil {
			return library.ComicBookMatcher{}, false, fmt.Errorf("rule %+v: %w", r, err)
		}
		children = append(children, child)
	}
	if len(children) == 0 {
		return library.ComicBookMatcher{}, false, nil
	}
	return groupMatcher(normalizeMode(mode), false, children...), true, nil
}

func isBoolGetterField(field string) bool {
	def, ok := builtinFields[field]
	return ok && def.BoolGetter != nil
}

func normalizeMode(mode string) string {
	if strings.EqualFold(mode, "Or") {
		return "Or"
	}
	return "And"
}

// Matches reports whether book satisfies rs's rules. Rules with no
// existing MatcherType equivalent (ComicInfoIsDirty, EnableProposed,
// HasBeenOpened, HasBeenRead, ComicBookIsDirty - see fields.go) are
// evaluated directly against the book; every other rule goes through the
// existing smart-list matcher engine via a synthetic one-book library, so
// AND/OR combination, negation, and the OR-group expansions
// (IsAnyOf/StartsWithAnyOf/GreaterEq/LesserEq - see translate.go) are all
// handled by already-tested code, not reimplemented here.
//
// A ruleset with zero rules matches every book - mirrors "no conditions
// means no filter" rather than being treated as an error, since dataman.dat
// group-level <filtersanddefaults> elements are always empty in the user's
// real file and must not block traversal into that group's children.
func (rs Ruleset) Matches(book *library.ComicBook) (bool, error) {
	if len(rs.Rules) == 0 {
		return true, nil
	}

	var results []bool

	for _, r := range rs.Rules {
		if !isBoolGetterField(r.Field) {
			continue
		}
		result, err := evalBoolGetterRule(r, book)
		if err != nil {
			return false, err
		}
		results = append(results, result)
	}

	group, ok, err := BuildConditionGroup(rs.Rules, rs.Mode)
	if err != nil {
		return false, err
	}
	if ok {
		// Group matchers (including the OR-groups TranslateRule builds
		// for IsAnyOf/StartsWithAnyOf/GreaterEq/LesserEq) are only
		// evaluated recursively via MatchBooks -> evaluateMatcher;
		// NewMatcherFromXML explicitly rejects a top-level group. Wrap
		// book in a throwaway one-book library and a synthetic smart
		// list to reach that path without duplicating its logic here.
		lib := &library.ComicLibrary{Books: []library.ComicBook{*book}}
		list := &library.ComicListItem{
			Type:        "ComicSmartListItem",
			MatcherMode: normalizeMode(rs.Mode),
			Matchers:    []library.ComicBookMatcher{group},
		}
		matched, err := lib.MatchBooks(list)
		if err != nil {
			return false, fmt.Errorf("evaluating conditions: %w", err)
		}
		results = append(results, len(matched) > 0)
	}

	return combine(results, rs.Mode), nil
}

func combine(results []bool, mode string) bool {
	if len(results) == 0 {
		return true
	}
	if normalizeMode(mode) == "Or" {
		for _, r := range results {
			if r {
				return true
			}
		}
		return false
	}
	for _, r := range results {
		if !r {
			return false
		}
	}
	return true
}

func evalBoolGetterRule(r Rule, book *library.ComicBook) (bool, error) {
	def, ok := builtinFields[r.Field]
	if !ok || def.BoolGetter == nil {
		return false, fmt.Errorf("field %q: not a BoolGetter field", r.Field)
	}
	mod := strings.ToLower(r.Modifier)
	if mod != "is" && mod != "not" {
		return false, fmt.Errorf("field %q: unsupported modifier %q", r.Field, r.Modifier)
	}
	wantYes := strings.EqualFold(strings.TrimSpace(r.Value), "yes") || strings.EqualFold(strings.TrimSpace(r.Value), "true")
	if mod == "not" {
		wantYes = !wantYes
	}
	actual := def.BoolGetter(book)
	return actual == wantYes, nil
}
