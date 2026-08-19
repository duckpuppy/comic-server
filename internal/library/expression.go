package library

import (
	"reflect"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// matchExpression evaluates a MatcherTypeExpression matcher's compiled
// Starlark expression against a book. This is a best-effort approximation of
// ComicRack's Python expression matcher (see ComicBookExpressionMatcher):
// simple field access, comparisons, boolean logic, and string methods work
// as-is, but Python stdlib imports and other CPython-only constructs do not.
//
// A parse or eval failure (unparseable expression, unknown identifier,
// non-boolean result, etc.) mirrors ComicRack's own error handling: the
// matcher simply never matches rather than aborting the whole smart list.
func (m *Matcher) matchExpression(book *ComicBook) bool {
	if m.compiledExpr == nil {
		return false
	}

	thread := &starlark.Thread{Name: "smartlist-expression"}
	var opts syntax.FileOptions
	val, err := starlark.EvalExprOptions(&opts, thread, m.compiledExpr, buildExpressionGlobals(book))
	if err != nil {
		return false
	}

	result := val.Truth() == starlark.True

	// ComicBookExpressionMatcher.MatchBook: operator 1 ("is false") negates
	// the raw expression result; any other operator (0, "is true") does not.
	if m.Operator == 1 {
		return !result
	}
	return result
}

// buildExpressionGlobals exposes a ComicBook's exported fields as Starlark
// globals under their Go struct names, so expressions can reference them
// directly (e.g. `Rating > 3 and "X-Men" in Series`), matching how
// ComicRack's expression editor lets users type plain property names.
func buildExpressionGlobals(book *ComicBook) starlark.StringDict {
	env := starlark.StringDict{}
	v := reflect.ValueOf(*book)
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if val, ok := toStarlarkValue(v.Field(i)); ok {
			env[f.Name] = val
		}
	}
	return env
}

// toStarlarkValue converts a ComicBook field's reflected value into a
// Starlark value. Unsupported kinds (nested structs other than ComicTime,
// slices, maps) are skipped rather than erroring, since a book has fields an
// expression is unlikely to ever need.
func toStarlarkValue(v reflect.Value) (starlark.Value, bool) {
	switch v.Kind() {
	case reflect.String:
		return starlark.String(v.String()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(v.Int()), true
	case reflect.Float32, reflect.Float64:
		return starlark.Float(v.Float()), true
	case reflect.Bool:
		return starlark.Bool(v.Bool()), true
	case reflect.Struct:
		if ct, ok := v.Interface().(ComicTime); ok {
			if ct.IsZero() {
				return starlark.String(""), true
			}
			return starlark.String(ct.Format(time.RFC3339)), true
		}
		return nil, false
	default:
		return nil, false
	}
}
