package library

import (
	"regexp"
	"strconv"
	"strings"
)

// Free-text search with structured tokens (comic-server-mzkf), used by the
// Browse page's quick search. Tokens combine freely with plain words:
//
//	Batman V2017 #15 (2016)
//
//	#15      issue number equals 15 (leading zeros ignored, so #015 matches 15)
//	(2016)   year equals 2016
//	V2017    volume equals 2017 (case-insensitive; the V must start a word)
//
// Everything left over is plain words; every word must appear somewhere in
// the book's series, title or publisher (case-insensitive). Tokens and words
// are ANDed.

var (
	searchNumberRe = regexp.MustCompile(`#(\S+)`)
	searchYearRe   = regexp.MustCompile(`\((\d{4})\)`)
	searchVolumeRe = regexp.MustCompile(`(?i)(?:^|\s)v(\d+)\b`)
)

// SearchQuery is a parsed quick-search string.
type SearchQuery struct {
	Words  []string // lower-cased plain words
	Number string   // normalised issue number, "" if none
	Year   int      // 0 if none
	Volume int      // 0 if none
}

// ParseSearchQuery splits q into structured tokens and plain words.
func ParseSearchQuery(q string) SearchQuery {
	var sq SearchQuery

	if m := searchNumberRe.FindStringSubmatch(q); m != nil {
		sq.Number = normalizeIssueNumber(m[1])
		q = strings.Replace(q, m[0], " ", 1)
	}
	if m := searchYearRe.FindStringSubmatch(q); m != nil {
		sq.Year, _ = strconv.Atoi(m[1])
		q = strings.Replace(q, m[0], " ", 1)
	}
	if m := searchVolumeRe.FindStringSubmatch(q); m != nil {
		sq.Volume, _ = strconv.Atoi(m[1])
		q = strings.Replace(q, m[0], " ", 1)
	}

	sq.Words = strings.Fields(strings.ToLower(q))
	return sq
}

// IsEmpty reports whether the query filters nothing.
func (sq SearchQuery) IsEmpty() bool {
	return len(sq.Words) == 0 && sq.Number == "" && sq.Year == 0 && sq.Volume == 0
}

// Match reports whether book satisfies every token and word.
func (sq SearchQuery) Match(book *ComicBook) bool {
	if sq.Number != "" && normalizeIssueNumber(book.Number) != sq.Number {
		return false
	}
	if sq.Year != 0 && book.Year != sq.Year {
		return false
	}
	if sq.Volume != 0 && book.Volume != sq.Volume {
		return false
	}
	if len(sq.Words) == 0 {
		return true
	}
	hay := strings.ToLower(book.Series + " " + book.Title + " " + book.Publisher)
	for _, w := range sq.Words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// normalizeIssueNumber strips leading zeros so "015" and "15" compare equal
// and lower-cases the rest ("12A" == "12a"). All-zero input becomes "0".
func normalizeIssueNumber(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	trimmed := strings.TrimLeft(n, "0")
	switch {
	case n != "" && trimmed == "":
		return "0"
	case trimmed != "" && (trimmed[0] < '0' || trimmed[0] > '9') && trimmed != n:
		// "0a" stays "0a" rather than collapsing to a bare "a".
		return "0" + trimmed
	}
	return trimmed
}
