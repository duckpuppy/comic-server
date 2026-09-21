package cbl

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// MatchPath records which strategy resolved (or failed to resolve) a CBL
// entry against the library - see spec §2a/§2b/§2c
// (docs/plans/2026-09-20-cbl-reading-list-import.md).
type MatchPath int

const (
	// MatchNone means no book in the library matched this entry.
	MatchNone MatchPath = iota
	// MatchCVID means the entry's <Database Name="cv"> issue ID matched
	// a book's comicvine_issue custom value directly (spec §2a).
	MatchCVID
	// MatchSeriesNumber means the entry matched via the ComicRackCE-parity
	// Series/Number/Volume/Year/Format fallback (spec §2b).
	MatchSeriesNumber
	// MatchManual means a user corrected this entry via the
	// match-correction UI (spec §2d, comic-server-a2hz) - re-pointing it
	// to a different book than MatchEntry picked, or confirming one of
	// the tied candidates by hand. Never produced by MatchEntry itself;
	// set by the storage layer when applying a correction.
	MatchManual
)

func (p MatchPath) String() string {
	switch p {
	case MatchCVID:
		return "cv_id"
	case MatchSeriesNumber:
		return "series_number"
	case MatchManual:
		return "manual"
	default:
		return "none"
	}
}

// Match is the result of matching one CBL Book against the library.
type Match struct {
	Entry Book
	Path  MatchPath
	Book  *library.ComicBook // nil if Path == MatchNone

	// Candidates is the final narrowed candidate set MatchEntry was
	// choosing among when Path == MatchSeriesNumber (spec §2d - "which
	// candidates were considered"). Length 1 means the match was
	// unambiguous; length > 1 means step 7's FirstOrDefault tie-break
	// picked Book (= Candidates[0]) arbitrarily among real ties - exactly
	// the case the match-correction UI exists for. Always nil for
	// MatchCVID (a direct ID lookup, not a heuristic) and MatchNone
	// (nothing to have narrowed).
	Candidates []*library.ComicBook
}

// rxVolumeInName and rxSpecial replicate ComicRackCE's ComicInfo.cs
// exactly (rxVolume, rxSpecial) - re-read that source if these ever need
// changing, do not hand-tune independently of it. Source:
// ../ComicRackCE/ComicRack.Engine/ComicInfo.cs:123,125.
var (
	rxVolumeInName = regexp.MustCompile(`(?i)\bv(ol(ume)?)?\.?\s?\d+\b\s*`)
	rxSpecial      = regexp.MustCompile(`(?i)[^a-z0-9]|\bthe\b|\band\b`)
)

// seriesEquals mirrors ComicInfo.SeriesEquals(a, b, options) exactly.
// ignoreVolume corresponds to CompareSeriesOptions.IgnoreVolumeInName,
// stripDown to CompareSeriesOptions.StripDown - both may be set together
// (the third and loosest widening pass in the reference algorithm).
func seriesEquals(a, b string, ignoreVolume, stripDown bool) bool {
	if ignoreVolume {
		a = strings.TrimSpace(rxVolumeInName.ReplaceAllString(a, ""))
		b = strings.TrimSpace(rxVolumeInName.ReplaceAllString(b, ""))
	}
	if stripDown {
		a = rxSpecial.ReplaceAllString(a, "")
		b = rxSpecial.ReplaceAllString(b, "")
	}
	return strings.EqualFold(a, b)
}

// MatchEntry matches one CBL Book against library, replicating
// ComicRackCE's ComicIdListItem.CreateFromReadingList algorithm
// (../ComicRackCE/ComicRack.Engine/Database/ComicIdListItem.cs:154-233 -
// re-read that source before changing this, do not work from memory of
// this comment alone). comic-server adds the CV-ID path (step 0, spec
// §2a) ahead of ComicRack's own Id/FileName/string-matching steps, since
// ComicRack's own format predates the <Database> extension and can't use
// it.
//
// library is every candidate book to match against (typically the whole
// library, or a caller-filtered subset).
func MatchEntry(entry Book, books []*library.ComicBook) Match {
	// Step 0 (comic-server addition, spec §2a): CV issue ID, exact.
	if cvID, ok := entry.CVIssueID(); ok {
		for _, b := range books {
			if issueID, ok := cvIssueID(b); ok && issueID == cvID {
				return Match{Entry: entry, Path: MatchCVID, Book: b}
			}
		}
	}

	// Steps 1-7 (ComicRackCE parity, spec §2b). comic-server has no
	// per-library-instance internal book GUID shared with a stranger's
	// CBL (step 1 in the reference algorithm only ever fires reimporting
	// your OWN previously-exported list) and FileName is rarely set in
	// the wild (step 2) - both are skipped here since comic-server's
	// import path doesn't carry ComicRack's own internal Ids across
	// libraries; the CV-ID path above already covers the "reimporting
	// something we recognize" case better than either would.
	candidates := filterByNumberAndSeries(books, entry.Number, entry.Series, false, false)
	if len(candidates) == 0 {
		candidates = filterByNumberAndSeries(books, entry.Number, entry.Series, true, false)
	}
	if len(candidates) == 0 {
		candidates = filterByNumberAndSeries(books, entry.Number, entry.Series, true, true)
	}

	if len(candidates) > 1 {
		if narrowed := filterByYear(candidates, entry.Year); len(narrowed) > 0 {
			candidates = narrowed
		}
	}
	if len(candidates) > 1 {
		if narrowed := filterByVolume(candidates, entry.Volume); len(narrowed) > 0 {
			candidates = narrowed
		}
	}
	if len(candidates) > 1 && entry.Format != "" {
		if narrowed := filterByFormat(candidates, entry.Format); len(narrowed) > 0 {
			candidates = narrowed
		}
	}

	if len(candidates) == 0 {
		return Match{Entry: entry, Path: MatchNone}
	}
	// FirstOrDefault() tie-break, matching the reference implementation
	// exactly (spec §2b step 7) - an arbitrary pick on a still-ambiguous
	// set is a known fuzziness in ComicRack itself, not something
	// comic-server needs to solve better by default. Candidates carries
	// the full narrowed set (not just its length) so the match-correction
	// UI (spec §2d, comic-server-a2hz) can offer the other candidates a
	// user can re-point to instead of Book, when this pick is wrong.
	return Match{Entry: entry, Path: MatchSeriesNumber, Book: candidates[0], Candidates: candidates}
}

func cvIssueID(b *library.ComicBook) (int, bool) {
	v, ok := library.GetCustomValue(b.CustomValuesStore, "comicvine_issue")
	if !ok || v == "" {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func filterByNumberAndSeries(books []*library.ComicBook, number, series string, ignoreVolume, stripDown bool) []*library.ComicBook {
	var out []*library.ComicBook
	for _, b := range books {
		if b.Number == number && seriesEquals(b.Series, series, ignoreVolume, stripDown) {
			out = append(out, b)
		}
	}
	return out
}

func filterByYear(books []*library.ComicBook, year int) []*library.ComicBook {
	var out []*library.ComicBook
	for _, b := range books {
		diff := b.Year - year
		if diff < 0 {
			diff = -diff
		}
		if diff <= 1 {
			out = append(out, b)
		}
	}
	return out
}

func filterByVolume(books []*library.ComicBook, volume int) []*library.ComicBook {
	var out []*library.ComicBook
	for _, b := range books {
		if b.Volume == volume {
			out = append(out, b)
		}
	}
	return out
}

func filterByFormat(books []*library.ComicBook, format string) []*library.ComicBook {
	var out []*library.ComicBook
	for _, b := range books {
		if strings.EqualFold(b.Format, format) {
			out = append(out, b)
		}
	}
	return out
}
