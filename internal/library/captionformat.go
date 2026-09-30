package library

import (
	"fmt"
	"regexp"
	"strings"
)

// This file is a port of ComicRackCE's ExtendedStringFormater
// (cYo.Common/Text/ExtendedStringFormater.cs), the template language behind
// VirtualTag caption formats. The control flow (including the "success"
// bookkeeping that decides whether an [optional] group is shown) mirrors the
// C# source line for line on purpose: a template written for ComicRack has to
// produce the same output here.
//
// Grammar:
//
//	{Field}          substitute a book field (see captionFieldValue)
//	{Field:format}   same, run through a .NET-style numeric format (e.g. 0000)
//	[ ... ]          optional group: dropped when a {Field} or $function<>
//	                 directly inside it evaluates empty
//	\X               literal X
//	$name<a,b,...>   function call (see captionfuncs.go)

// captionError is panicked internally for malformed templates and failing
// functions, and recovered in formatCaption into ComicRack's "#ERROR - msg"
// output.
type captionError struct{ msg string }

func (e captionError) Error() string { return e.msg }

func captionFail(format string, args ...any) {
	panic(captionError{msg: fmt.Sprintf(format, args...)})
}

// formatCaption expands a caption format against getValue. getValue returns
// the text for a field name and whether the field exists (a missing field is
// ComicRack's null, which expands to empty).
//
// Like ExtendedStringFormater.Format, a malformed template or a failing
// function yields "#ERROR - <message>" rather than an error return.
func formatCaption(format string, getValue func(name string) (string, bool)) (result string) {
	defer func() {
		if r := recover(); r != nil {
			ce, ok := r.(captionError)
			if !ok {
				panic(r)
			}
			result = "#ERROR - " + ce.msg
		}
	}()
	s, _ := formatCaptionRunes([]rune(format), getValue, false)
	return s
}

// formatCaptionRunes is the recursive worker. The bool is ComicRack's
// "success" flag: true while no empty {Field}/$function has been seen at this
// level (nested [] groups can flip it back to true, exactly as in the C#
// source - `success |= success2`).
func formatCaptionRunes(format []rune, getValue func(string) (string, bool), escapeComma bool) (string, bool) {
	var sb strings.Builder
	success := true
	for i := 0; i < len(format); i++ {
		c := format[i]
		switch c {
		case '[':
			part := captionGetPart(format, &i, '[', ']', nil)
			v, ok := formatCaptionRunes([]rune(part), getValue, escapeComma)
			if ok {
				sb.WriteString(v)
			}
			success = success || ok
		case '$':
			part := captionGetPart(format, &i, '$', '>', []rune{'<', '>', '$'})
			inner, _ := formatCaptionRunes([]rune(part), getValue, true)
			res := captionParseFunction(inner)
			sb.WriteString(res)
			success = success && res != ""
		case '{':
			part := captionGetPart(format, &i, '{', '}', nil)
			v := captionFormatValue(part, getValue)
			success = success && v != ""
			if escapeComma && strings.Contains(v, ",") {
				v = strings.ReplaceAll(v, ",", `\,`)
			}
			sb.WriteString(v)
		case '\\':
			i++
			if i >= len(format) {
				captionFail("trailing backslash in format")
			}
			sb.WriteRune(format[i])
		default:
			sb.WriteRune(c)
		}
	}
	return sb.String(), success
}

// captionGetPart returns the text between a matching open/close pair starting
// at format[*index], leaving *index on the closing character (ExtendedString
// Formater.GetPart). Unbalanced input is an error, as Substring throws in C#.
func captionGetPart(text []rune, index *int, openChar, closeChar rune, charToEscape []rune) string {
	start, end, depth := 0, 0, 0
	sameChar := openChar == closeChar
	for *index < len(text) {
		c := text[*index]
		var n rune
		if *index+1 < len(text) {
			n = text[*index+1]
		}
		if c == '\\' && runeIn(charToEscape, n) {
			*index += 2
			continue
		}
		if c == openChar && (!sameChar || depth <= 0) {
			if depth == 0 {
				start = *index + 1
			}
			depth++
		} else if c == closeChar {
			depth--
			if depth == 0 {
				end = *index
				break
			}
		}
		*index++
	}
	if end < start {
		captionFail("unbalanced %c%c in format", openChar, closeChar)
	}
	return string(text[start:end])
}

func runeIn(set []rune, r rune) bool {
	for _, s := range set {
		if s == r {
			return true
		}
	}
	return false
}

// captionFormatValue resolves one {Field} / {Field:spec} token.
func captionFormatValue(token string, getValue func(string) (string, bool)) string {
	parts := strings.Split(token, ":")
	val, ok := getValue(parts[0])
	if !ok {
		return ""
	}
	if len(parts) == 1 {
		return val
	}
	// ComicRack converts the string value to a double when it can, then
	// applies .NET string.Format("{0:spec}"); a non-numeric value ignores the
	// spec and is returned as-is.
	f, isNum := parseCaptionNumber(val)
	if !isNum {
		return val
	}
	if out, ok := formatDotNetNumber(f, parts[1]); ok {
		return out
	}
	return val
}

var captionFuncNameRe = regexp.MustCompile(`^([^<]+?)<(.+)$`)

// captionParseFunction splits "name<a,b,c" (the text between $ and the final
// >) into a function name and unescaped-comma-separated arguments, then runs it.
func captionParseFunction(value string) string {
	m := captionFuncNameRe.FindStringSubmatch(value)
	name, params := "", ""
	if m != nil {
		name, params = strings.ToLower(m[1]), m[2]
	}
	var args []string
	if params != "" {
		for _, a := range splitUnescapedCommas(params) {
			args = append(args, strings.ReplaceAll(strings.TrimSpace(a), `\,`, ","))
		}
	}
	return runCaptionFunction(name, args)
}

// splitUnescapedCommas splits on commas not preceded by a backslash (the
// .NET pattern (?<!\\), - Go's RE2 has no lookbehind).
func splitUnescapedCommas(s string) []string {
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' && (i == 0 || s[i-1] != '\\') {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	return append(out, s[last:])
}
