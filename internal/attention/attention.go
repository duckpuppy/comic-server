// Package attention computes "needs attention" flags for library books
// (comic-server-z77p): named, severity-ranked problems such as a missing
// series name or a file that still needs converting to CBZ.
//
// FlagsFor derives flags from the book record alone. Flags that need other
// data live elsewhere: file_missing comes from the API's disk pass, and the
// ComicVine-cache and sibling flags are in crosscheck.go.
package attention

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Severity ranks a flag. Higher sorts first.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarn:
		return "warn"
	default:
		return "info"
	}
}

// Flag identifies one kind of problem.
type Flag string

const (
	FlagIsPDF            Flag = "is_pdf"
	FlagIsZip            Flag = "is_zip"
	FlagNeedsConversion  Flag = "needs_conversion"
	FlagCoverOnly        Flag = "cover_only"
	FlagNoSeries         Flag = "no_series"
	FlagLowPageCount     Flag = "low_page_count"
	FlagUnknownPublisher Flag = "unknown_publisher"
	FlagMissingFormat    Flag = "missing_format"
	FlagMissingCount     Flag = "missing_count"
	// FlagFileMissing needs a disk check, so FlagsFor never returns it; the
	// API layer adds it from its background pass (comic-server-4q1z).
	FlagFileMissing Flag = "file_missing"
)

// LowPageCountThreshold: books with fewer pages than this (but more than 0,
// which means "not counted yet") are flagged.
const LowPageCountThreshold = 5

// coverOnlyMaxPages is the most pages a "cover"-named file can have and
// still count as a lone cover rather than a comic.
const coverOnlyMaxPages = 3

// Info describes a flag for display.
type Info struct {
	Flag     Flag
	Label    string
	Hint     string
	Severity Severity
}

// Catalog lists every flag. Order is the tie-break when severity and count
// are equal.
var Catalog = []Info{
	{FlagFileMissing, "File missing", "The archive is not on disk where the library says. If many books show here, check the path mapping first.", SeverityError},
	{FlagMetadataMismatch, "Wrong ComicVine issue?", "The tagged ComicVine issue has a different number, or belongs to a different volume than the book's comicvine_volume. Re-scrape the book, or fix its number.", SeverityWarn},
	{FlagCVVolumeConflict, "ComicVine volume conflict", "Books with the same series, volume and format are tagged with different ComicVine volumes. The ones listed disagree with most of their siblings. Re-scrape them.", SeverityWarn},
	{FlagNoSeries, "No series", "Book has no series name. Scrape it or set one by hand.", SeverityWarn},
	{FlagIsPDF, "PDF files", "PDFs cannot be converted to CBZ or read by all devices.", SeverityWarn},
	{FlagNeedsConversion, "Needs conversion", "Archive is not a CBZ (CBR, CB7, CBT...). Convert it.", SeverityWarn},
	{FlagIsZip, "Plain .zip", "A .zip that should be renamed or converted to .cbz.", SeverityWarn},
	{FlagCoverOnly, "Cover only", "Tiny file with \"cover\" in its name - probably a stray cover image.", SeverityWarn},
	{FlagLowPageCount, "Low page count", "Fewer than 5 pages. May be truncated or not a real issue.", SeverityInfo},
	{FlagUnknownPublisher, "Unknown publisher", "Publisher is empty.", SeverityInfo},
	{FlagMissingFormat, "Missing format", "Format (Series, Annual, One-Shot...) is empty.", SeverityInfo},
	{FlagMissingCount, "Missing issue count", "Series has no total issue count.", SeverityInfo},
	{FlagCountMismatch, "Count differs from ComicVine", "Issue count is not ComicVine's. Fill Issue Counts updates counts it set itself; a count you set by hand is yours to check.", SeverityInfo},
}

var infoByFlag = func() map[Flag]Info {
	m := make(map[Flag]Info, len(Catalog))
	for _, i := range Catalog {
		m[i.Flag] = i
	}
	return m
}()

// Lookup returns the catalog entry for a flag.
func Lookup(f Flag) (Info, bool) {
	i, ok := infoByFlag[f]
	return i, ok
}

// FlagsFor returns the flags that apply to one book.
func FlagsFor(b *library.ComicBook) []Flag {
	var out []Flag
	ext := strings.ToLower(filepath.Ext(b.FilePath))

	switch ext {
	case ".pdf":
		out = append(out, FlagIsPDF)
	case ".zip":
		out = append(out, FlagIsZip)
	case ".cbr", ".cb7", ".cbt", ".rar", ".7z", ".tar":
		out = append(out, FlagNeedsConversion)
	}

	if b.PageCount > 0 {
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(b.FilePath, `\`, "/")))
		if b.PageCount <= coverOnlyMaxPages && strings.Contains(base, "cover") {
			out = append(out, FlagCoverOnly)
		} else if b.PageCount < LowPageCountThreshold {
			out = append(out, FlagLowPageCount)
		}
	}

	if strings.TrimSpace(b.Series) == "" {
		out = append(out, FlagNoSeries)
	} else if b.Count <= 0 {
		// A missing count only means something for a named series.
		out = append(out, FlagMissingCount)
	}
	if strings.TrimSpace(b.Publisher) == "" {
		out = append(out, FlagUnknownPublisher)
	}
	if strings.TrimSpace(b.Format) == "" {
		out = append(out, FlagMissingFormat)
	}
	return out
}

// Summary is one flag's count for the dashboard card.
type Summary struct {
	Flag     Flag
	Label    string
	Hint     string
	Severity Severity
	Count    int
}

// Summarize ranks flags by severity (errors first), then count (largest
// first), then catalog order. Flags with a zero count are omitted.
func Summarize(counts map[Flag]int) []Summary {
	var out []Summary
	order := map[Flag]int{}
	for i, info := range Catalog {
		order[info.Flag] = i
		if n := counts[info.Flag]; n > 0 {
			out = append(out, Summary{info.Flag, info.Label, info.Hint, info.Severity, n})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return order[a.Flag] < order[b.Flag]
	})
	return out
}
