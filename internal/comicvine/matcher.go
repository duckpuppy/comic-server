package comicvine

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Confidence levels for a match result.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// MatchResult is a scored candidate volume match for a comic book, along
// with the best matching issue found within that volume (if any).
type MatchResult struct {
	Volume     Volume
	Score      float64
	Confidence string
	Issue      *Issue
}

var (
	reNonWord   = regexp.MustCompile(`\W+`)
	reApostro   = regexp.MustCompile(`'`)
	reGiantSize = regexp.MustCompile(`(?i)giant[- ]*sized?`)
	reKingSize  = regexp.MustCompile(`(?i)king[- ]*sized?`)
	reOneShot   = regexp.MustCompile(`(?i)one[- ]*shot`)
	reNonNum    = regexp.MustCompile(`[^\d.-]+`)
)

// mirror publishers are international reprint houses that are rarely the
// desired primary match, ported from comic-vine-scraper's matchscore.py.
var mirrorPublishers = map[string]bool{
	"marvel italia": true,
	"marvel uk":     true,
	"semic_as":      true,
	"abril":         true,
}

// ScoreVolumes scores each candidate volume against the parsed book metadata
// and returns the results sorted by score descending, with confidence assigned.
func ScoreVolumes(parsed ParsedFilename, candidates []Volume, priorVolumeID int) []MatchResult {
	currentYear := time.Now().Year()
	results := make([]MatchResult, 0, len(candidates))
	for _, v := range candidates {
		seriesYear := parseVolumeYear(v.StartYear)
		score := bookScore(parsed.IssueNumber, v.CountOfIssues) +
			nameScore(parsed.Series, v.Name) +
			fuzzyNameScore(parsed.Series, v.Name) +
			publisherScore(v.Publisher.Name) +
			priorScore(v.ID, priorVolumeID) +
			yearScore(parsed.Year, seriesYear, currentYear) +
			recencyScore(seriesYear, currentYear)
		results = append(results, MatchResult{Volume: v, Score: score})
	}
	assignConfidence(results)
	return results
}

// SelectIssue picks the issue matching issueNumber from issues, trying an
// exact string match first and falling back to a normalized comparison
// (leading zeros, fractional issue notation).
func SelectIssue(issueNumber string, issues []Issue) *Issue {
	if issueNumber == "" || len(issues) == 0 {
		return nil
	}

	for i := range issues {
		if issues[i].IssueNumber == issueNumber {
			return &issues[i]
		}
	}

	normalized := normalizeIssueNumber(issueNumber)
	for i := range issues {
		if normalizeIssueNumber(issues[i].IssueNumber) == normalized {
			return &issues[i]
		}
	}
	return nil
}

func assignConfidence(results []MatchResult) {
	if len(results) == 0 {
		return
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })

	if results[0].Score < 0 {
		for i := range results {
			results[i].Confidence = ConfidenceLow
		}
		return
	}

	if len(results) == 1 {
		results[0].Confidence = ConfidenceHigh
	} else {
		gap := results[0].Score - results[1].Score
		switch {
		case gap >= 10:
			results[0].Confidence = ConfidenceHigh
		case gap >= 3:
			results[0].Confidence = ConfidenceMedium
		default:
			results[0].Confidence = ConfidenceLow
		}
	}
	for i := 1; i < len(results); i++ {
		results[i].Confidence = ConfidenceLow
	}
}

// splitNameWords tokenizes a series/book name the same way comic-vine-scraper's
// matchscore.py does, so word-by-word comparisons line up.
func splitNameWords(name string) []string {
	name = strings.ToLower(name)
	name = reApostro.ReplaceAllString(name, "")
	name = reNonWord.ReplaceAllString(name, " ")
	name = reGiantSize.ReplaceAllString(name, "giant size")
	name = reKingSize.ReplaceAllString(name, "king size")
	name = reOneShot.ReplaceAllString(name, "one shot")
	return strings.Fields(name)
}

// nameScore is +5 per matching word between the book and series names,
// -1 per unmatched book word, and -1 per leftover series word.
func nameScore(bookName, seriesName string) float64 {
	bookWords := splitNameWords(bookName)
	seriesWords := splitNameWords(seriesName)

	score := 0.0
	for _, w := range bookWords {
		if idx := indexOf(seriesWords, w); idx >= 0 {
			score += 5
			seriesWords = append(seriesWords[:idx], seriesWords[idx+1:]...)
		} else {
			score--
		}
	}
	score -= float64(len(seriesWords))
	return score
}

func indexOf(words []string, w string) int {
	for i, s := range words {
		if s == w {
			return i
		}
	}
	return -1
}

// fuzzyNameScore is an enhancement over the original plugin: a small bonus
// based on overall string similarity, so typos and minor naming differences
// don't tank an otherwise-good match.
func fuzzyNameScore(bookName, seriesName string) float64 {
	a := strings.ToLower(strings.TrimSpace(bookName))
	b := strings.ToLower(strings.TrimSpace(seriesName))
	if a == "" || b == "" {
		return 0
	}
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 0
	}
	similarity := 1.0 - float64(levenshtein(a, b))/float64(maxLen)
	return similarity * 2
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	return min(a, b, c)
}

func priorScore(volumeID, priorVolumeID int) float64 {
	if priorVolumeID != 0 && volumeID == priorVolumeID {
		return 7
	}
	return 0
}

func publisherScore(publisher string) float64 {
	pub := strings.ToLower(publisher)
	if strings.Contains(pub, "panini") || strings.Contains(pub, "deagostina") || mirrorPublishers[pub] {
		return -6
	}
	return 0
}

func bookScore(issueNumberStr string, countOfIssues int) float64 {
	num := parseBookNumber(issueNumberStr)
	if countOfIssues > 100 {
		return 100
	}
	if num-1 <= float64(countOfIssues) {
		return 100
	}
	return -100
}

func parseBookNumber(s string) float64 {
	if s == "" {
		return -1000
	}
	cleaned := reNonNum.ReplaceAllString(s, "")
	f, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return -999
	}
	return f
}

func isValidYear(y, currentYear int) bool {
	return y > 1900 && y <= currentYear+1
}

func yearScore(bookYear, seriesYear, currentYear int) float64 {
	if !isValidYear(bookYear, currentYear) {
		return 0
	}
	if !isValidYear(seriesYear, currentYear) {
		return -100
	}
	if seriesYear > bookYear {
		return -500
	}
	return 0
}

func recencyScore(seriesYear, currentYear int) float64 {
	if isValidYear(seriesYear, currentYear) {
		return -float64(currentYear-seriesYear) / 100.0
	}
	return -1.0
}

func parseVolumeYear(s string) int {
	s = strings.TrimSpace(s)
	y, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return y
}

func normalizeIssueNumber(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case "½", "1/2":
		return "0.5"
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f == math.Trunc(f) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.ToLower(s)
}

// Score adjustments applied by cover verification — roughly equivalent to
// two word matches worth of namescore, enough to break close ties without
// overriding a strong text-based mismatch.
const (
	coverMatchBoost      = 20.0
	coverMismatchPenalty = -20.0
)

// ApplyCoverVerification adjusts each result's score based on how closely
// its cover art matches the local comic's cover (coverHashes maps volume CV
// ID to a previously downloaded/cached cover hash), then re-sorts and
// reassigns confidence. Candidates with no known cover hash are left
// unchanged, so cover verification only ever sharpens an existing match.
func ApplyCoverVerification(results []MatchResult, localHash CoverHash, coverHashes map[int]CoverHash) []MatchResult {
	for i := range results {
		hash, ok := coverHashes[results[i].Volume.ID]
		if !ok {
			continue
		}
		if localHash.Similarity(hash) >= CoverVerifyThreshold {
			results[i].Score += coverMatchBoost
		} else {
			results[i].Score += coverMismatchPenalty
		}
	}
	assignConfidence(results)
	return results
}

// AmbiguousByCover reports whether the top two candidates' own cover art is
// too similar to tell apart confidently — the classic single-issue-vs-its-
// own-TPB-collection confusion — even though their text scores differ.
func AmbiguousByCover(results []MatchResult, coverHashes map[int]CoverHash) bool {
	if len(results) < 2 {
		return false
	}
	h1, ok1 := coverHashes[results[0].Volume.ID]
	h2, ok2 := coverHashes[results[1].Volume.ID]
	if !ok1 || !ok2 {
		return false
	}
	return h1.Similarity(h2) >= AmbiguousCoverThreshold
}
