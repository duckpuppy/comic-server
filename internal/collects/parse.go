package collects

import (
	"regexp"
	"strings"
)

// Candidate is one run of issues found in a book's description text
// ("Collects GHOST RIDER (2006) #33-35"). It is only a guess: the title still
// has to be matched to a ComicVine volume, and a person confirms (tracker #40).
type Candidate struct {
	// Title is the series name as written, with the year and connector words
	// removed. Empty when the text gave a range but no title.
	Title string
	// Year is the "(2006)" next to the title, or "".
	Year string
	// From and To are issue numbers as text; To is empty for one issue.
	From, To string
}

// Parsed is what ParseSummary found.
type Parsed struct {
	Candidates []Candidate
	// Notes say why a result may be incomplete, in plain words.
	Notes []string
}

var (
	reLead = regexp.MustCompile(`(?i)\b(?:collects?|collecting|collected)\b`)
	// "#12", "#1-6", "#1–#4"; en and em dashes count.
	reRun = regexp.MustCompile(`#\s*(\d+(?:\.\d+)?)(?:\s*[-–—]\s*#?\s*(\d+(?:\.\d+)?))?`)
	// ", 591-594" straight after a run: more of the same title.
	reMore = regexp.MustCompile(`^\s*,\s*(?:and\s+)?(\d+(?:\.\d+)?)(?:\s*[-–—]\s*(\d+(?:\.\d+)?))?`)
	reYear = regexp.MustCompile(`\(\s*(\d{4})\s*\)`)
	// Words that join one run to the next or introduce a list.
	reConnector = regexp.MustCompile(`(?i)^(?:and|plus|also|with|including|includes|featuring|starting with|stories from|stories|material from|issues?|the best-selling|best-selling|the first|the)\b[\s,:;&]*`)
	reTrailing  = regexp.MustCompile(`(?i)[\s,;:&–—-]*(?:\band|\bplus)?[\s,;:&–—-]*$`)
)

const maxSummaryScan = 2000

// ParseSummary reads the "Collects ..." statement out of a description. It
// finds nothing (an empty result) when the text has no such statement or no
// "#" issue number after it. It stops at the end of the sentence that holds
// the list, so marketing text after it is not read as more issues.
func ParseSummary(text string) Parsed {
	var out Parsed
	loc := reLead.FindStringIndex(text)
	if loc == nil {
		return out
	}
	seg := text[loc[1]:]
	if len(seg) > maxSummaryScan {
		seg = seg[:maxSummaryScan]
	}

	var lastTitle, lastYear string
	pos := 0
	for pos < len(seg) {
		m := reRun.FindStringSubmatchIndex(seg[pos:])
		if m == nil {
			break
		}
		before := seg[pos : pos+m[0]]
		// A sentence break or a very long stretch before the next "#" means
		// the list is over and this is some other text.
		if len(out.Candidates) > 0 && (strings.Contains(before, ". ") || strings.Contains(before, "\n") || len(before) > 160) {
			break
		}
		title, year, incomplete := cleanTitle(before)
		if incomplete {
			out.addNote("the text says \"starting with\", so the list may be incomplete")
		}
		if title == "" {
			title, year = lastTitle, lastYear
		} else {
			lastTitle, lastYear = title, year
		}
		from := seg[pos+m[2] : pos+m[3]]
		to := ""
		if m[4] >= 0 {
			to = seg[pos+m[4] : pos+m[5]]
		}
		out.add(title, year, from, to)
		pos += m[1]

		for {
			mm := reMore.FindStringSubmatchIndex(seg[pos:])
			if mm == nil {
				break
			}
			to := ""
			if mm[4] >= 0 {
				to = seg[pos+mm[4] : pos+mm[5]]
			}
			out.add(title, year, seg[pos+mm[2]:pos+mm[3]], to)
			pos += mm[1]
		}
		rest := strings.TrimLeft(seg[pos:], " \t")
		if rest == "" || rest[0] == '.' && (len(rest) == 1 || rest[1] == ' ' || rest[1] == '\n' || rest[1] == '\r') {
			break
		}
	}
	for _, c := range out.Candidates {
		if c.Title == "" {
			out.addNote("a range has no series title in the text")
			break
		}
	}
	return out
}

func (p *Parsed) add(title, year, from, to string) {
	if to == from {
		to = ""
	}
	p.Candidates = append(p.Candidates, Candidate{Title: title, Year: year, From: from, To: to})
}

func (p *Parsed) addNote(n string) {
	for _, have := range p.Notes {
		if have == n {
			return
		}
	}
	p.Notes = append(p.Notes, n)
}

// cleanTitle turns the text in front of a "#" into a series title and year.
func cleanTitle(s string) (title, year string, incomplete bool) {
	if m := reYear.FindStringSubmatch(s); m != nil {
		year = m[1]
		s = reYear.ReplaceAllString(s, " ")
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeft(s, " ,;:.–—-")
	for {
		loc := reConnector.FindStringIndex(s)
		if loc == nil {
			break
		}
		if strings.HasPrefix(strings.ToLower(s), "starting with") {
			incomplete = true
		}
		s = s[loc[1]:]
	}
	s = reTrailing.ReplaceAllString(s, "")
	return strings.TrimSpace(s), year, incomplete
}
