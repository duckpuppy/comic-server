// Package relink finds where a missing comic file went (comic-server-heox).
//
// It only ever looks at the names in one directory - the folder the book's
// file used to be in - so a relink costs one directory listing, never a walk
// of the library (the disks are shared with media that may be streaming).
// Idea from ComicManager's Path Audit "Relink" (ideas only).
package relink

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

// Outcome says how a search ended.
type Outcome string

const (
	// Found: exactly one candidate; Match holds its file name.
	Found Outcome = "found"
	// NotFound: nothing in the folder fits.
	NotFound Outcome = "not_found"
	// Ambiguous: more than one file fits, so none is picked.
	Ambiguous Outcome = "ambiguous"
)

// How a match was made.
const (
	ReasonSameName   = "same file name, different extension or case"
	ReasonSeriesAndN = "same series and issue number"
)

// Result is the answer of Find.
type Result struct {
	Outcome    Outcome
	Match      string   // file name (not a path) when Outcome == Found
	Reason     string   // how it was matched, when Found
	Candidates []string // every fitting name when Ambiguous
}

var comicExts = map[string]bool{
	".cbz": true, ".cbr": true, ".cb7": true, ".cbt": true, ".zip": true, ".rar": true, ".7z": true, ".pdf": true,
}

// IsComicFile reports whether name has a comic archive extension.
func IsComicFile(name string) bool { return comicExts[strings.ToLower(filepath.Ext(name))] }

// Find picks the one file in names (the directory listing) that a missing
// book most likely became. oldName is the book's old file name. taken reports
// a name another book already uses; those never match. Strategies, in order,
// each needing exactly one candidate:
//  1. same name ignoring case and extension (an archive converted or renamed
//     in place);
//  2. the file name parses to the same series and issue number.
//
// A strategy with two or more fits is Ambiguous: it is reported, never
// guessed, and the next strategy is not tried.
func Find(oldName string, book *library.ComicBook, names []string, taken func(name string) bool) Result {
	stem := strings.ToLower(strings.TrimSuffix(oldName, filepath.Ext(oldName)))

	var avail []string
	for _, n := range names {
		if !IsComicFile(n) || n == oldName {
			continue
		}
		if taken != nil && taken(n) {
			continue
		}
		avail = append(avail, n)
	}

	var same []string
	for _, n := range avail {
		if strings.ToLower(strings.TrimSuffix(n, filepath.Ext(n))) == stem {
			same = append(same, n)
		}
	}
	if r, done := pick(same, ReasonSameName); done {
		return r
	}

	wantSeries := library.CompressedSeriesName(book.Series)
	wantNum := normalizeNumber(book.Number)
	if wantSeries == "" || wantNum == "" {
		return Result{Outcome: NotFound}
	}
	var fits []string
	for _, n := range avail {
		p := comicvine.ParseFilename(n)
		if p.Series == "" || p.IssueNumber == "" {
			continue
		}
		if library.CompressedSeriesName(p.Series) == wantSeries && normalizeNumber(p.IssueNumber) == wantNum {
			fits = append(fits, n)
		}
	}
	if r, done := pick(fits, ReasonSeriesAndN); done {
		return r
	}
	return Result{Outcome: NotFound}
}

func pick(found []string, reason string) (Result, bool) {
	switch len(found) {
	case 0:
		return Result{}, false
	case 1:
		return Result{Outcome: Found, Match: found[0], Reason: reason}, true
	default:
		return Result{Outcome: Ambiguous, Candidates: found}, true
	}
}

// normalizeNumber makes "001", "1" and "#1" compare equal; "1.5" stays.
func normalizeNumber(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if s == "" {
		return ""
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.ToLower(s)
}

// SplitRawPath splits a library path into its directory part (with the
// trailing separator, in the path's own style) and file name. Library paths
// may be Windows-style on a Linux host, so both separators count.
func SplitRawPath(raw string) (dirWithSep, name string) {
	i := strings.LastIndexAny(raw, `/\`)
	return raw[:i+1], raw[i+1:]
}
