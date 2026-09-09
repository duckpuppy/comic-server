// Package libraryorganizer ports ComicRack's "Library Organizer" plugin
// (libraryorganizer.py/locommon.py/lobookmover.py) into comic-server -
// see comic-server-3bz for the full design record: the real losettingsx.dat
// profile shapes, the trust-boundary decision (comic-server's first
// feature that moves/renames the user's OWN existing comic files, not
// just a new file it generated itself), and the preview-then-apply
// safety model.
package libraryorganizer

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// templateFieldRegex mirrors PathMaker.template_regex from lobookmover.py
// exactly: {prefix<name(args)>postfix} - the whole bracketed group is
// dropped if the field resolves empty, prefix/postfix only appear
// alongside a non-empty result. Ground-truthed against the real plugin
// source, not guessed.
var templateFieldRegex = regexp.MustCompile(`\{(?P<prefix>[^{}<]*)<(?P<name>[^\d\s(>]*)(?P<args>\d*|(?:\([^)]*\))*)>(?P<postfix>[^{}]*)\}`)

// FieldResolver returns the raw (un-illegal-char-cleaned) text value of
// one named template field for a book, or ok=false if this build doesn't
// support that field name. Takes profile too - "month" needs the
// profile's configurable month-name table (losettingsx.dat's <Months>
// block lets the user rename them), matching PathMaker.insert_month_as_name
// reading self.profile.Months. Numeric fields return unpadded text;
// PathMaker.padNumeric applies zero-padding centrally so every numeric
// field shares one implementation.
type FieldResolver func(book *library.ComicBook, profile Profile) (string, bool)

// numericField marks a resolver's result as eligible for bare-digit
// padding args (e.g. <number2>), matching lobookmover.py's
// insert_number_field path - a plain text field's args (if any) are
// simply invalid instead.
type fieldDef struct {
	resolve FieldResolver
	numeric bool
}

// fieldTable is PathMaker.template_to_field's Go counterpart, scoped to
// the fields the user's REAL configured profiles (Default/Share/Archive/
// Move To 0Day, ground-truthed against their actual losettingsx.dat)
// actually reference: publisher, imprint, series, volume, format, month,
// year, number, count. The full ComicRack field vocabulary is much
// larger (Writer/Genre/multi-value fields with a "get every value across
// the whole series" mode, Counter with persistent cross-run state,
// FirstIssueNumber/LastIssueNumber needing an "earliest/last book in this
// series" lookup, ReadPercentage's bucketing) - deliberately not ported
// here since none of it appears in real usage; a template referencing an
// unsupported field name fails closed (see insertField's "unknown field"
// path) rather than silently producing wrong output.
var fieldTable = map[string]fieldDef{
	"publisher": {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return b.Publisher, true }},
	"imprint":   {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return b.Imprint, true }},
	"series":    {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return shadowSeries(b), true }},
	"title":     {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return shadowTitle(b), true }},
	"format":    {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return shadowFormat(b), true }},
	"volume":    {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return intFieldText(shadowVolume(b)), true }, numeric: true},
	"year":      {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return intFieldText(shadowYear(b)), true }, numeric: true},
	"number":    {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return shadowNumber(b), true }, numeric: true},
	"count":     {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return intFieldText(shadowCount(b)), true }, numeric: true},
	// "month" (name form) looks up the profile's configurable month name;
	// "month#" is the plain numeric month - matches
	// insert_text_field's own "Month and not template_name.endswith('#')"
	// split. Both map to the same underlying ComicBook.Month int, so both
	// entries exist here rather than one shared definition.
	"month":  {resolve: monthName},
	"month#": {resolve: func(b *library.ComicBook, p Profile) (string, bool) { return intFieldText(b.Month), true }, numeric: true},
}

func monthName(b *library.ComicBook, p Profile) (string, bool) {
	name, ok := p.Months[b.Month]
	if !ok {
		return "", true
	}
	return name, true
}

// intFieldText mirrors the Python's own "-1/empty means blank" convention
// for ComicRack's unset-integer sentinel.
func intFieldText(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// shadowSeries/shadowTitle/shadowFormat/shadowVolume/shadowNumber/
// shadowCount/shadowYear mirror ComicRackCE's own ComicBook.ShadowXxx
// properties (ComicRack.Engine/ComicBook.cs) - the real field UNLESS
// EnableProposed is set and the real field is empty, in which case it
// falls back to the matching Proposed* value (ComicVine scrape results
// pending manual review/acceptance - see comic-server-3x3's still-open
// "Proposed Values" pipeline stage). comic-server's own ComicVine
// integration commits scraped values directly rather than staging them
// as Proposed*, so EnableProposed is never meaningfully true for a
// comic-server-managed book today - these simplify to the real field
// unconditionally. Revisit if comic-server-3x3 ever adds Proposed*
// support.
func shadowSeries(b *library.ComicBook) string { return b.Series }
func shadowTitle(b *library.ComicBook) string  { return b.Title }
func shadowFormat(b *library.ComicBook) string { return b.Format }
func shadowVolume(b *library.ComicBook) int    { return b.Volume }
func shadowNumber(b *library.ComicBook) string { return b.Number }
func shadowCount(b *library.ComicBook) int     { return b.Count }
func shadowYear(b *library.ComicBook) int      { return b.Year }

// Months maps a ComicBook.Month value (1-16; 13-16 are ComicRack's
// quarterly-special names) to its display name - profile-configurable in
// the real plugin (losettingsx.dat's <Months> block), not hardcoded, since
// a user can rename them.
type Months map[int]string

// DefaultMonths is the stock month-name table every real profile in the
// user's losettingsx.dat uses unmodified.
var DefaultMonths = Months{
	1: "January", 2: "February", 3: "March", 4: "April", 5: "May", 6: "June",
	7: "July", 8: "August", 9: "September", 10: "October", 11: "November", 12: "December",
	13: "Spring", 14: "Summer", 15: "Fall", 16: "Winter",
}

// IllegalCharacters is a single-character substitution map applied to
// every generated path segment - profile-configurable (losettingsx.dat's
// <IllegalCharacters>), not hardcoded to Windows' reserved character set,
// since ComicRack lets the user pick the replacement text per character.
type IllegalCharacters map[string]string

// DefaultIllegalCharacters is the stock substitution table every real
// profile in the user's losettingsx.dat uses unmodified.
var DefaultIllegalCharacters = IllegalCharacters{
	`"`: "'", "/": "", "*": "", "<": "[", "?": "", ">": "]", "|": "!", ":": " - ", `\`: "",
}

// Profile is the subset of a Library Organizer profile PathMaker needs -
// see comic-server-3bz.2 for the full profile shape (BaseFolder, Mode,
// ExcludeRules, etc) once profile storage lands; PathMaker itself only
// needs the template-expansion inputs.
type Profile struct {
	Months                Months
	IllegalCharacters     IllegalCharacters
	ReplaceMultipleSpaces bool
	EmptyFolder           string // fallback text for a folder segment that resolves empty
}

// multipleSpaceRegex mirrors the Python's own `\s\s+` collapse.
var multipleSpaceRegex = regexp.MustCompile(`\s\s+`)

// MakeFolderPath expands a FolderTemplate into its cleaned, backslash-
// delimited path segments (NOT yet joined to a base folder or an OS path
// separator - the caller decides that, since "join to BaseFolder" and
// "which OS separator" are apply-time/profile concerns, not template-
// engine concerns). Mirrors PathMaker.make_folder_path: each
// backslash-delimited segment of the raw expanded template is illegal-
// -char-cleaned and trailing-period-stripped INDEPENDENTLY, not the whole
// path at once - a publisher name containing "/" only pollutes its own
// segment.
func MakeFolderPath(book *library.ComicBook, template string, profile Profile) ([]string, bool) {
	template = strings.Trim(strings.TrimSpace(template), `\`)
	if template == "" {
		return nil, true
	}

	rough, ok := insertFieldsIntoTemplate(template, book, profile)
	segments := strings.Split(rough, `\`)
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		if strings.TrimSpace(seg) == "" {
			seg = profile.EmptyFolder
		}
		seg = replaceIllegalCharacters(seg, profile.IllegalCharacters)
		seg = strings.TrimRight(strings.TrimSpace(seg), ".")
		if profile.ReplaceMultipleSpaces {
			seg = multipleSpaceRegex.ReplaceAllString(seg, " ")
		}
		out = append(out, seg)
	}
	return out, ok
}

// MakeFileName expands a FileTemplate into a cleaned file name WITHOUT
// extension (the caller appends the source file's real extension, or
// profile.FilelessFormat for a fileless book - an apply-time concern, not
// a template-engine one). Mirrors PathMaker.make_file_name.
func MakeFileName(book *library.ComicBook, template string, profile Profile) (string, bool) {
	name, ok := insertFieldsIntoTemplate(template, book, profile)
	name = strings.TrimSpace(name)
	name = replaceIllegalCharacters(name, profile.IllegalCharacters)
	if profile.ReplaceMultipleSpaces {
		name = multipleSpaceRegex.ReplaceAllString(name, " ")
	}
	return name, ok
}

func replaceIllegalCharacters(text string, table IllegalCharacters) string {
	// Longest-key-first, matching the Python's
	// `sorted(..., key=len, reverse=True)` - not load-bearing for the
	// real table (every key is a single character today) but preserved
	// for correctness if a profile ever configures a multi-character
	// illegal sequence.
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sortByLenDesc(keys)
	for _, k := range keys {
		text = strings.ReplaceAll(text, k, table[k])
	}
	return text
}

func sortByLenDesc(keys []string) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && len(keys[j-1]) < len(keys[j]); j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
}

// insertFieldsIntoTemplate repeatedly substitutes every recognized
// {prefix<name(args)>postfix} group until none remain or a full pass
// makes no further progress (mirrors insert_fields_into_template's own
// "stop once every remaining match is already known-invalid" loop guard,
// which prevents an infinite loop on a template containing literal,
// unresolvable brace syntax). ok is false if ANY field in the template
// was unrecognized - the caller (preview, comic-server-3bz.4) surfaces
// that as a flagged failure rather than silently emitting the literal
// {<unknownfield>} text into a real file path.
func insertFieldsIntoTemplate(template string, book *library.ComicBook, profile Profile) (string, bool) {
	ok := true
	for {
		matches := templateFieldRegex.FindAllStringSubmatchIndex(template, -1)
		if len(matches) == 0 {
			break
		}
		invalidCount := 0
		next := templateFieldRegex.ReplaceAllStringFunc(template, func(m string) string {
			sub := templateFieldRegex.FindStringSubmatch(m)
			replacement, valid := insertField(sub, book, profile)
			if !valid {
				invalidCount++
				ok = false
			}
			return replacement
		})
		if invalidCount == len(matches) {
			// Every remaining match is unresolvable - stop rather than
			// loop forever re-matching the same invalid text.
			template = next
			break
		}
		if next == template {
			break
		}
		template = next
	}
	return template, ok
}

// insertField mirrors PathMaker.insert_field for the plain
// (non-inversion, non-conditional) form only - see the package doc
// comment on comic-server-3bz.1's scoping of '!'/'?' template syntax.
// sub is templateFieldRegex's submatch slice: [full, prefix, name, args, postfix].
func insertField(sub []string, book *library.ComicBook, profile Profile) (result string, ok bool) {
	prefix, name, args, postfix := sub[1], sub[2], sub[3], sub[4]

	if strings.HasPrefix(name, "!") || strings.HasPrefix(name, "?") {
		// Inversion/conditional syntax - not implemented (see package
		// doc comment). Left as literal text, flagged invalid so the
		// caller knows this template isn't fully supported rather than
		// silently producing a path with the raw template syntax baked
		// into it.
		return sub[0], false
	}

	def, known := fieldTable[name]
	if !known {
		return sub[0], false
	}

	value, valid := def.resolve(book, profile)
	if !valid {
		return sub[0], false
	}

	if def.numeric && args != "" {
		if width, err := strconv.Atoi(args); err == nil {
			value = padNumeric(value, width)
		}
	}

	if value == "" {
		return "", true
	}
	return prefix + value + postfix, true
}

// padNumeric mirrors PathMaker.pad: zero-pads a numeric string's integer
// portion to width, preserving a decimal remainder and a leading minus
// sign. A non-numeric value (e.g. an issue number like "1A") passes
// through unpadded, matching the Python's own ValueError fallback -
// comic-server's Number field already has pseudo-numeric comparison
// logic elsewhere (smartlist.go) for exactly this "not a plain float"
// case, but padding a non-numeric string has no sensible meaning, so this
// just leaves it alone rather than reusing that machinery.
func padNumeric(value string, width int) string {
	if width <= 0 {
		return value
	}
	neg := strings.HasPrefix(value, "-")
	unsigned := strings.TrimPrefix(value, "-")
	intPart, frac, hasFrac := strings.Cut(unsigned, ".")
	if _, err := strconv.Atoi(intPart); err != nil {
		return value
	}
	for len(intPart) < width {
		intPart = "0" + intPart
	}
	out := intPart
	if hasFrac {
		out += "." + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}
