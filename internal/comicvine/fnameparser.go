package comicvine

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ParsedFilename struct {
	Series      string
	IssueNumber string
	Year        int
	Volume      int
}

func ParseFilename(filename string) ParsedFilename {
	name := strings.TrimSpace(filename)
	if name == "" {
		return ParsedFilename{}
	}
	name = filepath.Base(name)

	// strip file extension (but not if dot is at position 0)
	if i := strings.LastIndex(name, "."); i > 0 {
		ext := strings.ToLower(name[i:])
		switch ext {
		case ".cbz", ".cbr", ".cb7", ".cbt", ".pdf", ".zip":
			name = name[:i]
		}
	}

	// strip leading "nnn" reading-list prefix when followed by "#" issue marker
	if m := reLeadingNumWithHash.FindStringSubmatch(name); m != nil {
		name = m[2]
	}

	// strip embedded issue title after "series #xxx - title ..."
	if m := reEmbeddedTitle.FindStringSubmatch(name); m != nil {
		name = m[1] + " " + m[2]
	}

	result, ok := extract(name)
	if !ok || strings.TrimSpace(result.Series) == "" {
		return ParsedFilename{Series: name}
	}
	return result
}

func ParseFilenameWithRegex(filename, pattern string) *ParsedFilename {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(filename)
	name = filepath.Base(name)
	if i := strings.LastIndex(name, "."); i > 0 {
		ext := strings.ToLower(name[i:])
		switch ext {
		case ".cbz", ".cbr", ".cb7", ".cbt", ".pdf", ".zip":
			name = name[:i]
		}
	}

	match := re.FindStringSubmatch(name)
	if match == nil {
		return nil
	}

	result := &ParsedFilename{}
	for i, gname := range re.SubexpNames() {
		if i == 0 || match[i] == "" {
			continue
		}
		val := strings.TrimSpace(match[i])
		if val == "" {
			continue
		}
		switch gname {
		case "series":
			result.Series = val
		case "num":
			result.IssueNumber = val
		case "year":
			if y, err := strconv.Atoi(val); err == nil {
				result.Year = y
			}
		}
	}
	if result.Series == "" {
		return nil
	}
	return result
}

var (
	reLeadingNumWithHash = regexp.MustCompile(`^\s*(\d+)[\s._-]+([^#]+?#-?\d+.*)`)
	reEmbeddedTitle      = regexp.MustCompile(`^((?:[a-zA-Z,.\-]+\s+)+#?(?:\d+[.0-9]*))\s*(?:-).*?((\(.*)?)$`)
	reParenBrackets      = regexp.MustCompile(`\([^(]*?\)`)
	reCurlyBrackets      = regexp.MustCompile(`\{[^{]*?\}`)
	reSquareBrackets     = regexp.MustCompile(`\[[^\[]*?\]`)
	reUnderscore         = regexp.MustCompile(`_`)
	reVolume             = regexp.MustCompile(`(?i)(\b((v|vol)\.?|volume))\s*-?\s*[0-9]+[.0-9a-z]*`)
	rePageCount          = regexp.MustCompile(`(?i)\b[.,]?\s*\d+\s*(p|pg|pgs|pages)\b[.,]?`)
	reCoverCount         = regexp.MustCompile(`(?i)(\d+\s*of\s*\d+\s*covers)`)
	reOfN                = regexp.MustCompile(`(?i)(?:\d)(\s*(of|de|di|von|van|z)\s*#*\d+)`)
	reRangeSuffix        = regexp.MustCompile(`(\d)(-\d+)`)
	reIssueNumber        = regexp.MustCompile(`(^|[_\s#])(-?\d*\.?\d\w*)`)
	reReadingListDot     = regexp.MustCompile(`^\s*\d+\.\s+`)
	reReadingListDash    = regexp.MustCompile(`^\s*\d+\s*-\s*`)
	reLeadingZeros       = regexp.MustCompile(`^(0+)([0-9].*)$`)
	reNumericOnly        = regexp.MustCompile(`^-?[.0-9]+$`)
	reWhitespace         = regexp.MustCompile(`\s{2,}`)

	reVYear       = regexp.MustCompile(`(?i)(^|[, \-_])v(\d{4})($|[, \-_])`)
	reParenInner  = regexp.MustCompile(`\([^[\](){}]*?\)`)
	reCurlyInner  = regexp.MustCompile(`\{[^[\](){}]*?\}`)
	reSquareInner = regexp.MustCompile(`\[[^[\](){}]*?\]`)
	reYearRange   = regexp.MustCompile(`(\d{4})\s*-\s*\d{1,4}`)
	reYear4       = regexp.MustCompile(`^\d{4}$`)

	re2000AD = regexp.MustCompile(`(?i)^\s*2000[\s.\-_]*a[\s.\-_]*d`)
	reBeano  = regexp.MustCompile(`(?i)^\s*the[\s.\-_]+beano[\s.\-_]+#?\d{4}`)
)

func extract(name string) (ParsedFilename, bool) {
	s := name

	yearStr := extractYear(s)
	year := 0
	if yearStr != "" {
		year, _ = strconv.Atoi(yearStr)
	}

	// strip bracketed content recursively
	s = recurseSub(reParenBrackets, s)
	s = recurseSub(reCurlyBrackets, s)
	s = recurseSub(reSquareBrackets, s)

	// clean underscores
	s = reUnderscore.ReplaceAllString(s, " ")

	// remove volume indicators
	s = reVolume.ReplaceAllString(s, "")

	// remove page counts
	s = rePageCount.ReplaceAllString(s, "")

	// remove "X of Y covers"
	s = reCoverCount.ReplaceAllString(s, "")

	// remove "X of Y" in multiple languages, and range suffixes like "583-586"
	s = replaceOfN(s)
	s = replaceRangeSuffix(s)

	// if name uses dashes instead of spaces, convert dashes to spaces
	if strings.Contains(s, "-") && !strings.Contains(s, " ") {
		s = replaceDashes(s)
	}

	// extract issue numbers
	matches := extractNumbers(s)

	// strip reading-list prefix if multiple numbers and starts with "05. " or "12 - "
	if len(matches) > 1 {
		if reReadingListDot.MatchString(s) {
			s = reReadingListDot.ReplaceAllString(s, "")
			matches = extractNumbers(s)
		} else if m := reReadingListDash.FindString(s); m != "" {
			rest := s[len(m):]
			if len(rest) > 0 && (rest[0] < '0' || rest[0] > '9') {
				s = rest
				matches = extractNumbers(s)
			}
		}
	}

	issueNum := ""
	series := s

	if len(matches) > 0 {
		last := matches[len(matches)-1]
		issueNum = last.number
		series = s[:last.fullStart] + s[last.fullEnd:]

		// strip leading zeros
		if m := reLeadingZeros.FindStringSubmatch(issueNum); m != nil {
			issueNum = m[2]
		}
		// normalize numeric values
		if reNumericOnly.MatchString(issueNum) {
			issueNum = normalizeNumber(issueNum)
		}
	}

	// contract whitespace and strip junk from ends
	series = reWhitespace.ReplaceAllString(series, " ")
	series = strings.Trim(series, " ,-_")

	return ParsedFilename{
		Series:      series,
		IssueNumber: issueNum,
		Year:        year,
	}, true
}

type numberMatch struct {
	number    string
	fullStart int
	fullEnd   int
	numStart  int
}

func extractNumbers(s string) []numberMatch {
	allIdx := reIssueNumber.FindAllStringSubmatchIndex(s, -1)
	var matches []numberMatch
	for _, idx := range allIdx {
		// group 1 (prefix) is idx[2]:idx[3], group 2 (number) is idx[4]:idx[5]
		numStr := s[idx[4]:idx[5]]
		matches = append(matches, numberMatch{
			number:    numStr,
			fullStart: idx[0],
			fullEnd:   idx[1],
			numStart:  idx[4],
		})
	}

	is2000AD := re2000AD.MatchString(s)
	isBeano := reBeano.MatchString(s)

	if !is2000AD && !isBeano {
		filtered := matches[:0]
		for _, m := range matches {
			if isYear(m.number) {
				// keep if preceded by '#' — check the prefix character
				if m.numStart > 0 && s[m.numStart-1] == '#' {
					filtered = append(filtered, m)
					continue
				}
				continue
			}
			filtered = append(filtered, m)
		}
		matches = filtered
	}

	return matches
}

func extractYear(s string) string {
	// type one: V2003 format
	allMatches := reVYear.FindAllStringSubmatch(s, -1)
	var years []string
	for _, m := range allMatches {
		if isYear(m[2]) {
			years = append(years, m[2])
		}
	}
	if len(years) == 1 {
		return years[0]
	}

	// type two: year inside brackets
	var results []string
	for _, re := range []*regexp.Regexp{reParenInner, reSquareInner, reCurlyInner} {
		results = append(results, re.FindAllString(s, -1)...)
	}

	// strip brackets and spaces
	var candidates []string
	for _, r := range results {
		r = strings.Trim(r, "()[]{}")
		r = strings.TrimSpace(r)
		// strip year ranges: "2006-2009" -> "2006"
		r = reYearRange.ReplaceAllString(r, "$1")
		if isYear(r) {
			candidates = append(candidates, r)
		}
	}

	if len(candidates) > 0 {
		return candidates[len(candidates)-1]
	}
	return ""
}

func isYear(s string) bool {
	if !reYear4.MatchString(s) {
		return false
	}
	y, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	return y > 1900 && y < 2100
}

func recurseSub(re *regexp.Regexp, s string) string {
	for re.MatchString(s) {
		s = re.ReplaceAllString(s, "")
	}
	return s
}

func replaceOfN(s string) string {
	// need to handle lookbehind: the digit before "of N" stays
	// Python: (?<=\d)(\s*(of|de|di|von|van|z)\s*#*\d+)
	return reOfN.ReplaceAllStringFunc(s, func(match string) string {
		// keep the leading digit, remove the rest
		return match[:1]
	})
}

func replaceRangeSuffix(s string) string {
	// Python: (?<=\d)(-\d+) — keep the digit, remove -N
	return reRangeSuffix.ReplaceAllString(s, "$1")
}

func replaceDashes(s string) string {
	// replace dashes that aren't preceded by [-_# ]
	var result strings.Builder
	for i, ch := range s {
		if ch == '-' && i > 0 {
			prev := s[i-1]
			if prev != '-' && prev != '_' && prev != '#' && prev != ' ' {
				result.WriteRune(' ')
				continue
			}
		}
		result.WriteRune(ch)
	}
	return result.String()
}

func normalizeNumber(s string) string {
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		// format without unnecessary trailing zeros
		result := strconv.FormatFloat(f, 'f', -1, 64)
		return result
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s
	}
	return strconv.FormatInt(i, 10)
}
