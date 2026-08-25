// Package scaninfo ports ComicRack's "ScanInformationFromFilename" plugin:
// detecting a scan-group/scanner tag from a comic's filename and merging it
// into the book's ScanInformation field. Ported from the real plugin source
// (ScanInformationFromFilename.py, GPL-licensed ComicRack plugin) rather
// than reinvented, so comic-server produces the same tags the user's
// ComicRack install already has for 65K+ books - see comic-server-pkk.1.
//
// The original's extraction regex relies on .NET lookahead
// ((?!...)), which Go's stdlib regexp (RE2) cannot express - RE2
// deliberately excludes backtracking constructs to guarantee linear-time
// matching. This package uses github.com/dlclark/regexp2, a pure-Go engine
// with .NET-compatible syntax, specifically so the ported patterns can stay
// byte-for-byte identical to the original rather than being approximated.
package scaninfo

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/dlclark/regexp2"
)

// Detector holds the compiled matching state for one set of scanners/
// blacklist/settings. Build once and reuse - compiling the regexes is the
// expensive part, matching against a filename is cheap.
type Detector struct {
	prefix  string
	unknown string

	scanners      *regexp2.Regexp // fallback: any known scanner name literally anywhere in the filename
	extract       *regexp2.Regexp // main heuristic: bracketed/underscore-delimited tag, blacklist-aware
	stripBlackout *regexp.Regexp  // strips embedded blacklist words out of a matched multi-word tag
}

// NewDetector compiles a Detector from the given scanner names and
// blacklist patterns (each blacklist entry is itself a regex fragment, as
// in the original scanners.txt/blacklist.txt - NOT a plain word list).
// prefix is prepended to every detected tag (e.g. "Scanner: "); unknown is
// the tag used when detection fails outright (empty string disables the
// fallback - see DetectTag).
func NewDetector(scanners, blacklist []string, prefix, unknown string) (*Detector, error) {
	if len(scanners) == 0 {
		return nil, fmt.Errorf("scaninfo: at least one scanner name is required")
	}

	// Sort scanners longest-first so e.g. "Clickwheel" matches before a
	// shorter accidental prefix like "cl" would - ported verbatim from the
	// original's `unformatedscanners.sort(key=len, reverse=True)`.
	sortedScanners := append([]string(nil), scanners...)
	sort.Slice(sortedScanners, func(i, j int) bool { return len(sortedScanners[i]) > len(sortedScanners[j]) })

	scannersPattern := "(?<Tags>" + strings.Join(sortedScanners, "|") + ")"
	scannersRe, err := regexp2.Compile(scannersPattern, regexp2.IgnoreCase)
	if err != nil {
		return nil, fmt.Errorf("scaninfo: compile scanners pattern: %w", err)
	}

	blacklistJoined := strings.Join(blacklist, "|")
	if blacklistJoined == "" {
		// Match-nothing alternative, so the surrounding pattern still
		// compiles even with an empty blacklist.
		blacklistJoined = "(?!)"
	}

	// NOTE: the original .NET pattern uses [^()\[\]\W\d_] here (require a
	// non-bracket, non-digit, non-underscore "word" char, i.e. a letter).
	// github.com/dlclark/regexp2 has a real bug combining a negated
	// shorthand class (\W) with a positive one (\d) inside one bracket
	// expression - \d silently gets dropped from the union, so
	// [^\W\d_] incorrectly matches digits (verified directly against the
	// library: [\W\d_] fails to match "2" even though \d is listed).
	// \p{L} (Unicode letter category) is the correct, simpler, and
	// Unicode-aware equivalent of "is a letter" and sidesteps the bug
	// entirely - same semantics, no [^...] combination involved.
	extractPattern := `(?:(?:__(?!.*__[^_]))|[(\[])(?!(?:` + blacklistJoined + `|[\s_\-\|/,])+[)\]])(?<Tags>(?=[^()\[\]]*\p{L})[^()\[\]]{2,})[)\]]?`
	extractRe, err := regexp2.Compile(extractPattern, regexp2.IgnoreCase)
	if err != nil {
		return nil, fmt.Errorf("scaninfo: compile extraction pattern: %w", err)
	}

	// The strip pattern only needs to find/remove blacklist words - no
	// lookahead required, so this one can stay on the fast stdlib engine.
	stripPattern := `(?i)(?:[^\w]|_|^)(?:` + regexpQuoteAlternation(blacklist) + `)(?:[^\w]|_|$)`
	stripRe, err := regexp.Compile(stripPattern)
	if err != nil {
		return nil, fmt.Errorf("scaninfo: compile strip pattern: %w", err)
	}

	return &Detector{
		prefix:        prefix,
		unknown:       unknown,
		scanners:      scannersRe,
		extract:       extractRe,
		stripBlackout: stripRe,
	}, nil
}

// regexpQuoteAlternation joins already-regex patterns as an alternation
// unchanged - blacklist entries are regex fragments, not literals, so they
// must NOT be quoted/escaped (matching the original, which joins them with
// "|" directly).
func regexpQuoteAlternation(patterns []string) string {
	if len(patterns) == 0 {
		return "(?:)"
	}
	return strings.Join(patterns, "|")
}

// Book is the minimal view of a comic book DetectTag needs. Callers
// populate OtherFields with every other populated text field on the book
// (comic-server's stand-in for ComicRack's GetComicFields() reflection) -
// used only for the "matched text is actually some other field's value"
// false-positive guard.
type Book struct {
	FilePath        string
	Series          string
	Title           string
	AlternateSeries string
	ScanInformation string
	OtherFields     []string // lowercased, for guard 1 (see DetectTag)
}

// DetectTag runs the full ported algorithm against one book and returns
// the tag to merge into ScanInformation (already prefixed) and whether a
// change should be made at all. A false ok means "leave ScanInformation
// alone" (mirrors the original's `continue` when nothing usable was found
// and no Unknown fallback is configured).
func (d *Detector) DetectTag(book Book) (tag string, ok bool) {
	filename := filepath.Base(book.FilePath)

	matches, err := findAllMatches(d.extract, filename)
	if err != nil {
		return "", false
	}

	var match *regexp2.Match
	unknownTag := ""

	if len(matches) == 0 {
		// Fallback: no bracket/underscore tag at all - check for a known
		// scanner name literally anywhere in the filename.
		m, _ := d.scanners.FindStringMatch(filename)
		if m == nil {
			if d.unknown == "" {
				return "", false
			}
			unknownTag = d.unknown
		} else {
			match = m
		}
	} else {
		match = matches[len(matches)-1]
	}

	// Guard 1: the matched tag text is actually the value of some OTHER
	// field on the book (e.g. a series subtitle in parens got matched
	// instead of a real scanner tag) - walk backwards through earlier
	// matches for one that isn't.
	if unknownTag == "" && match != nil {
		tagValue := strings.ToLower(strings.TrimSpace(match.GroupByName("Tags").String()))
		if containsFold(book.OtherFields, tagValue) {
			found := false
			for i := len(matches) - 2; i >= 0; i-- {
				candidate := strings.ToLower(strings.TrimSpace(matches[i].GroupByName("Tags").String()))
				if !containsFold(book.OtherFields, candidate) {
					match = matches[i]
					found = true
					break
				}
			}
			if !found {
				if d.unknown == "" {
					return "", false
				}
				unknownTag = d.unknown
			}
		}
	}

	// Guard 2: the matched tag is literally the parenthetical content of
	// the book's own Series/Title/AlternateSeries (e.g. "Batman (Vol 2)"
	// misread as a scan tag "Vol 2") - reject it outright.
	if unknownTag == "" && match != nil {
		tagValue := strings.ToLower(strings.TrimSpace(match.GroupByName("Tags").String()))
		for _, title := range []string{book.Series, book.Title, book.AlternateSeries} {
			if inner, found := parenthetical(title); found && strings.ToLower(inner) == tagValue {
				if d.unknown == "" {
					return "", false
				}
				unknownTag = d.unknown
				break
			}
		}
	}

	var newTag string
	if unknownTag != "" {
		newTag = d.prefix + unknownTag
	} else {
		cleaned := strings.Trim(match.GroupByName("Tags").String(), "_, ")
		cleaned = d.stripBlackout.ReplaceAllString(cleaned, "")
		newTag = d.prefix + cleaned
	}

	return newTag, true
}

// MergeTag adds newTag into the comma-separated ScanInformation list
// (deduplicated, alphabetically sorted, matching the original's behavior)
// and returns the merged value plus whether it actually changed anything.
func MergeTag(existing, newTag string) (merged string, changed bool) {
	var tags []string
	for t := range strings.SplitSeq(existing, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	if slices.Contains(tags, newTag) {
		return existing, false
	}

	tags = append(tags, newTag)
	sort.Strings(tags)
	return strings.Join(tags, ", "), true
}

func findAllMatches(re *regexp2.Regexp, s string) ([]*regexp2.Match, error) {
	var matches []*regexp2.Match
	m, err := re.FindStringMatch(s)
	if err != nil {
		return nil, err
	}
	for m != nil {
		matches = append(matches, m)
		m, err = re.FindNextMatch(m)
		if err != nil {
			return nil, err
		}
	}
	return matches, nil
}

func containsFold(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}

// parenthetical returns the content of the first "(...)" group in s, and
// whether one was found - ported from the original's
// `re.search(r"\((?P<match>.*)\)", title)`.
func parenthetical(s string) (string, bool) {
	start := strings.Index(s, "(")
	if start == -1 {
		return "", false
	}
	end := strings.LastIndex(s, ")")
	if end == -1 || end <= start {
		return "", false
	}
	return s[start+1 : end], true
}
