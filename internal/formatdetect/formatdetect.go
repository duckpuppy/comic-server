// Package formatdetect proposes a Format (Series, Limited Series, One-Shot,
// TPB, ...) for a book from its own record (comic-server-izc7). Idea from
// ComicManager's format step (ideas only): ordered rules, first match wins.
//
// Everything here is pure: Classify reads a book and returns a proposal,
// Plan decides whether a proposal may be applied. Callers do the writing.
package formatdetect

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Format names this package can assign. Hardcover, Absolute and Graphic
// Novel are manual-only: no rule here ever produces them.
const (
	FormatOmnibus       = "Omnibus"
	FormatTPB           = "TPB"
	FormatDirectorsCut  = "Director's Cut"
	FormatAnnual        = "Annual"
	FormatPreview       = "Preview"
	FormatBlackAndWhite = "Black and White"
	FormatOneShot       = "One-Shot"
	FormatLimited       = "Limited Series"
	FormatSeries        = "Series"
)

// Custom value keys. The comic_server_ prefix keeps them out of the CBZ
// metadata fingerprint.
const (
	// KeyAuto holds the Format this package last wrote. A Format that still
	// equals it is ours to re-detect; any other non-empty Format is manual.
	KeyAuto = "comic_server_format_auto"
	// KeyCount holds the issue Count used at the last detection.
	KeyCount = "comic_server_format_count"
	// KeyLocked set to "1" stops detection from ever touching the book.
	KeyLocked = "comic_server_format_locked"
)

const (
	omnibusPages = 500
	tpbPages     = 80
	limitedMax   = 15
)

var (
	omnibusRe    = regexp.MustCompile(`(?i)\bomnibus\b`)
	tpbRe        = regexp.MustCompile(`(?i)\b(tpb|trade paperback|collected edition)\b`)
	directorsRe  = regexp.MustCompile(`(?i)\bdirector'?s cut\b`)
	annualRe     = regexp.MustCompile(`(?i)\bannual\b`)
	previewRe    = regexp.MustCompile(`(?i)\bpreviews?\b`)
	blackWhiteRe = regexp.MustCompile(`(?i)\b(black\s*(and|&)\s*white|b\s*&\s*w)\b`)
	collectsRe   = regexp.MustCompile(`(?i)\bcollects?\b`)
	issueRangeRe = regexp.MustCompile(`#\s*\d+\s*[-\x{2013}\x{2014}]\s*#?\s*\d+`)
)

// Result is one proposal. Format is empty when no rule matched.
type Result struct {
	Format string
	Reason string
}

// Classify applies the ordered rules to book. First match wins.
func Classify(book *library.ComicBook) Result {
	names := book.Series + " " + book.Title
	withSummary := names + " " + book.Summary
	switch {
	case directorsRe.MatchString(names):
		return Result{FormatDirectorsCut, "name says director's cut"}
	case blackWhiteRe.MatchString(names):
		return Result{FormatBlackAndWhite, "name says black and white"}
	case omnibusRe.MatchString(withSummary):
		return Result{FormatOmnibus, "omnibus in name or summary"}
	case tpbRe.MatchString(withSummary):
		return Result{FormatTPB, "TPB in name or summary"}
	case book.PageCount >= tpbPages &&
		(collectsRe.MatchString(book.Summary) || issueRangeRe.MatchString(book.Summary)):
		return Result{FormatTPB, "80+ pages and summary lists collected issues"}
	case book.PageCount >= omnibusPages:
		return Result{FormatOmnibus, "500+ pages"}
	case annualRe.MatchString(names):
		return Result{FormatAnnual, "name says annual"}
	case previewRe.MatchString(names):
		return Result{FormatPreview, "name says preview"}
	case book.Count <= 0:
		return Result{} // no count: stays missing_format
	case book.Count > limitedMax:
		return Result{FormatSeries, "issue count is over 15"}
	case issueNumber(book.Number) > limitedMax:
		// Continuation / renumbered series: a high issue number beats a
		// small count.
		return Result{FormatSeries, "issue number is over 15 (continuation)"}
	case book.Count >= 2:
		return Result{FormatLimited, "issue count is 2-15"}
	}
	return Result{FormatOneShot, "issue count is 1"}
}

// issueNumber returns the leading integer of an issue number ("16", "16.1",
// "16a"), or 0 when there is none.
func issueNumber(s string) int {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0
	}
	return n
}

// Decision says what to do with one book.
type Decision int

const (
	// Skip: nothing to do (no file, locked, manual Format, or no rule matched
	// with nothing to clear).
	Skip Decision = iota
	// Set: write Proposal.Format.
	Set
)

// Proposal is the planned change for one book.
type Proposal struct {
	Decision Decision
	Old      string // Format now
	Format   string // Format to write (when Decision == Set)
	Reason   string
	Why      string // why Skip, for the preview
}

// Plan decides what detection may do to book. It never overwrites a manual
// Format: only an empty Format, or one still equal to what we last wrote.
func Plan(book *library.ComicBook) Proposal {
	p := Proposal{Old: book.Format}
	if library.IsWantedPlaceholder(book) {
		p.Why = "no file (wanted placeholder)"
		return p
	}
	if v, _ := library.GetCustomValue(book.CustomValuesStore, KeyLocked); v == "1" {
		p.Why = "locked"
		return p
	}
	cur := strings.TrimSpace(book.Format)
	auto, _ := library.GetCustomValue(book.CustomValuesStore, KeyAuto)
	if cur != "" && cur != auto {
		p.Why = "format set by hand"
		return p
	}
	r := Classify(book)
	if r.Format == "" {
		p.Why = "no rule matched"
		return p
	}
	if r.Format == cur {
		p.Why = "already " + cur
		return p
	}
	p.Decision, p.Format, p.Reason = Set, r.Format, r.Reason
	return p
}

// Apply writes a Set proposal onto book and records the auto marker and the
// count it was based on.
func Apply(book *library.ComicBook, p Proposal) {
	if p.Decision != Set {
		return
	}
	book.Format = p.Format
	store := library.SetCustomValue(book.CustomValuesStore, KeyAuto, p.Format)
	store = library.SetCustomValue(store, KeyCount, strconv.Itoa(book.Count))
	book.CustomValuesStore = store
}
