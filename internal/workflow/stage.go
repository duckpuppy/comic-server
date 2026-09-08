// Package workflow tracks each book's progress through comic-server's
// native replacement for the manual ComicRack ingest pipeline (comic-server
// -1iv): a fixed, ordered sequence of stages, each backed by a real
// comic-server feature, instead of a set of hand-maintained smart lists
// used as a todo list ("0 Day Folder", "01 Convert to CBZ", "02 To
// Scrape", ... "08 To Move").
//
// Design record (comic-server-1iv, settled with the user 2026-09-08):
//   - Stage is an EXPLICIT per-book field, not derived live from current
//     data shape on every read - it only advances forward via an explicit
//     transition (see AdvanceIfAtOrBefore), except for the one-time
//     backfill computed for books that already existed before this
//     feature shipped (see InferStage).
//   - Stored as a CustomValuesStore entry, not a new native XML element -
//     same extension point already used for comicvine_volume and Data
//     Manager's own markers, so ComicRack XML compatibility is
//     unaffected. The SQLite backend needs no schema change either: it
//     already decomposes every CustomValuesStore key into the
//     book_custom_values(key, value) index table.
//   - Only 5 stages are tracked (the other 4 original pipeline lists -
//     Day Folder, CVDBSKIP, Proposed Values, Duplicates Manager - are
//     tracked separately as comic-server-3x3, not part of this package).
//     ToMove is the terminal stage; comic-server-3bz (Library Organizer)
//     isn't built yet, so nothing currently advances a book OUT of ToMove
//   - it just sits there until that feature exists.
package workflow

import (
	"strings"

	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
)

// Stage is one step in the pipeline, in a fixed order (see Stages).
type Stage int

const (
	// StageUnknown is the zero value - never stored, only returned by
	// GetStage when the book has no workflow_stage custom value AND
	// hasn't been backfilled yet.
	StageUnknown Stage = iota
	StageConvertToCBZ
	StageScrape
	StageScanInfo
	StageDataManager
	StageToMove
)

// Stages is every real stage in pipeline order (excludes StageUnknown).
var Stages = []Stage{StageConvertToCBZ, StageScrape, StageScanInfo, StageDataManager, StageToMove}

// String returns the stable string form stored in CustomValuesStore -
// deliberately distinct from any display label so a future label wording
// change never touches on-disk data.
func (s Stage) String() string {
	switch s {
	case StageConvertToCBZ:
		return "convert_cbz"
	case StageScrape:
		return "scrape"
	case StageScanInfo:
		return "scan_info"
	case StageDataManager:
		return "data_manager"
	case StageToMove:
		return "to_move"
	default:
		return ""
	}
}

// Label is the human-readable form for UI display.
func (s Stage) Label() string {
	switch s {
	case StageConvertToCBZ:
		return "Convert to CBZ"
	case StageScrape:
		return "Scrape"
	case StageScanInfo:
		return "Scan Info"
	case StageDataManager:
		return "Data Manager"
	case StageToMove:
		return "To Move"
	default:
		return "Unknown"
	}
}

func parseStage(s string) Stage {
	for _, st := range Stages {
		if st.String() == s {
			return st
		}
	}
	return StageUnknown
}

// customValueKey is the CustomValuesStore key a book's stage is stored
// under. Plain snake_case, no namespace prefix, matching the existing
// convention (comicvine_volume, comicvine_issue) rather than inventing a
// new naming style.
const customValueKey = "comic_server_workflow_stage"

// GetStage reads book's explicitly stored stage, or StageUnknown if it
// has never been set (including for a book that predates this feature and
// hasn't been through the one-time backfill yet - see InferStage).
func GetStage(book *library.ComicBook) Stage {
	v, ok := library.GetCustomValue(book.CustomValuesStore, customValueKey)
	if !ok {
		return StageUnknown
	}
	return parseStage(v)
}

// SetStage writes stage onto book explicitly.
func SetStage(book *library.ComicBook, stage Stage) {
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, customValueKey, stage.String())
}

// InferStage computes a starting stage for a book that has never had one
// explicitly set - this is the one-time backfill logic
// (comic-server-1iv.1's CLI command). Never called again after a book has
// an explicit stage - see GetStage/SetStage.
//
// rulesets is the currently-configured Data Manager rule set (from
// datamanager.LoadRulesets) - the DataManager check runs it against the
// book and checks whether anything would still change, rather than
// looking for a specific "done" marker. That marker approach was tried
// first (checking for the "Data Manager processed" custom value some
// rules happen to write) and rejected after ground-truthing against the
// user's real ~66K-book library: only one narrow cleanup rule ever sets
// that value, so it flagged ~97% of an already-fully-processed library as
// "still needing Data Manager." Data Manager rules are idempotent - a
// book already conforming to the current rules produces zero changes
// when they're re-run - so "would anything change" is the accurate signal
// the marker approach was trying (and failing) to approximate.
func InferStage(book *library.ComicBook, rulesets []datamanager.Ruleset) Stage {
	if needsConvertToCBZ(book.FilePath) {
		return StageConvertToCBZ
	}
	if _, tagged := library.GetCustomValue(book.CustomValuesStore, "comicvine_volume"); !tagged {
		return StageScrape
	}
	if book.ScanInformation == "" {
		return StageScanInfo
	}
	if dataManagerWouldChange(book, rulesets) {
		return StageDataManager
	}
	return StageToMove
}

// dataManagerWouldChange runs rulesets against a COPY of book (never
// mutating the caller's book - InferStage is a read-only classification,
// not an apply) and reports whether anything would change. No configured
// rulesets means nothing is pending, not "unknown" - a book can't be
// stuck waiting on rules that don't exist.
func dataManagerWouldChange(book *library.ComicBook, rulesets []datamanager.Ruleset) bool {
	if len(rulesets) == 0 {
		return false
	}
	working := *book
	changes, err := datamanager.ApplyAll(&working, rulesets)
	if err != nil {
		// A rule error during classification shouldn't be treated as
		// "definitely still pending" or "definitely done" - err toward
		// still-pending (safer default: a book stays visible on the
		// dashboard rather than silently disappearing past a step that
		// actually failed to evaluate).
		return true
	}
	return len(changes) > 0
}

// needsConvertToCBZ mirrors cbzconvert.NeedsConversion's extension check
// without importing internal/cbzconvert, to avoid a dependency cycle risk
// (cbzconvert may reasonably want to import workflow later to call
// AdvanceIfAtOrBefore after a successful conversion - comic-server-1iv.2).
func needsConvertToCBZ(filePath string) bool {
	if filePath == "" {
		return false
	}
	ext := strings.ToLower(extOf(filePath))
	return ext != ".cbz" && ext != ".zip"
}

func extOf(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	base := p
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		base = p[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx > 0 {
		return base[idx:]
	}
	return ""
}

// AdvanceIfAtOrBefore moves book's stage forward to the step after
// completed, IF its current stage (computing via InferStage as a
// just-in-time backfill when it has never been explicitly set - rulesets
// is only consulted in that case, and only matters if the book's other
// conditions haven't already resolved its stage before reaching the Data
// Manager check) is at or before completed. Never moves a book backward
// and never skips past a stage it hasn't reached - a book already at
// StageDataManager running through the Scrape action again does not
// regress it to StageScrape. Returns whether the stage actually changed,
// so a caller can decide whether the book needs to be persisted.
func AdvanceIfAtOrBefore(book *library.ComicBook, completed Stage, rulesets []datamanager.Ruleset) bool {
	current := GetStage(book)
	if current == StageUnknown {
		current = InferStage(book, rulesets)
	}
	if current > completed {
		return false
	}

	next := nextStage(completed)
	if current == next {
		return false
	}
	SetStage(book, next)
	return true
}

// nextStage returns the stage after s, or s itself if s is already the
// terminal stage (StageToMove has nothing after it yet - comic-server-3bz
// isn't built).
func nextStage(s Stage) Stage {
	for i, st := range Stages {
		if st == s {
			if i+1 < len(Stages) {
				return Stages[i+1]
			}
			return s
		}
	}
	return s
}

// StageIndex returns s's 1-based position in Stages, or 0 for
// StageUnknown - useful for a UI ordering/progress display without
// exposing the underlying int enum values as meaningful numbers.
func StageIndex(s Stage) int {
	for i, st := range Stages {
		if st == s {
			return i + 1
		}
	}
	return 0
}

// cvdbSkipTag is the literal Tags value the user's real library uses to
// mark a book as intentionally exempt from ComicVine scraping - confirmed
// against their actual ComicDb.xml (`<Tags>CVDBSKIP</Tags>`), a real
// ComicRack-native Tags entry, not a comic-server custom value. Not a
// pipeline stage - it never advances, it's purely an informational count
// for the dashboard (comic-server-3x3 / comic-server-1iv.3): "how many
// books am I intentionally not scraping."
const cvdbSkipTag = "CVDBSKIP"

// IsCVDBSkip reports whether book carries the CVDBSKIP tag.
func IsCVDBSkip(book *library.ComicBook) bool {
	for tag := range strings.SplitSeq(book.Tags, ",") {
		if strings.TrimSpace(tag) == cvdbSkipTag {
			return true
		}
	}
	return false
}
