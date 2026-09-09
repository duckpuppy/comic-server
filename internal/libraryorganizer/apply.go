package libraryorganizer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// Mode selects whether Apply relocates the source file (Move - the
// original path is quarantined once the new one is safely written) or
// duplicates it (Copy - the original is left exactly where it was).
// Mirrors the real Library Organizer plugin's own per-profile Move/Copy
// setting (comic-server-3bz design decision: full multi-profile support,
// not just the user's Default/Move profile).
type Mode int

const (
	ModeMove Mode = iota
	ModeCopy
)

// ApplyOutcome reports what actually happened to one PlannedMove once
// Apply ran - distinct from PlannedMove's own Skipped/Failed/Collision,
// which are the PREVIEW-time verdict computed by Plan before anything is
// written. A move the preview approved can still fail here (e.g. a
// permission error, a source file that vanished between preview and
// apply) - FailReason explains why when that happens.
type ApplyOutcome struct {
	BookID string

	// Applied is true when a file was actually moved or copied on disk.
	Applied bool

	// NoOp is true when the book was already sitting at its planned
	// destination (PlannedMove.NewResolvedPath == OldResolvedPath) - no
	// file operation was needed, but the book still advances to
	// StageOrganized (self-healing backfill, see internal/workflow's
	// StageOrganized doc comment).
	NoOp bool

	// Skipped mirrors the PlannedMove's own Skipped/Failed/Collision -
	// Apply never touches a file for one of these, it just carries the
	// verdict through so a caller can render one combined result list.
	Skipped bool

	Failed     bool
	FailReason string
}

// ApplyOptions bundles Apply's non-move-specific inputs.
type ApplyOptions struct {
	Mode  Mode
	Trash *trash.Trash

	// Books maps BookID to the actual library record Apply should update
	// (FilePath and workflow stage) on success. A move whose BookID isn't
	// present in Books is treated as Failed - Apply never silently
	// no-ops an update it can't make.
	Books map[string]*library.ComicBook

	// Rulesets is passed through to workflow.AdvanceIfAtOrBefore for its
	// just-in-time backfill of a book that has never had an explicit
	// stage set - same parameter InferStage itself takes.
	Rulesets []datamanager.Ruleset
}

// Apply executes every move in moves that Plan approved (not Skipped, not
// Failed, not Collision) - anything Plan already flagged is carried
// through untouched, no file operation attempted. This is comic-server's
// first feature that moves or renames the user's own existing comic
// files, so every real write goes through internal/trash: Move uses
// WriteNew (new destination) + Quarantine (old path, not deleted) - the
// same pairing cbzconvert.Convert established for its own path-changing
// case; Copy uses WriteNew alone, since the source is left in place.
//
// On success, book.FilePath is updated to the move's NewRawPath and the
// book's workflow stage is advanced past StageToMove via
// workflow.AdvanceIfAtOrBefore - callers are responsible for persisting
// the updated books to the backend afterward, same division of
// responsibility as cbzconvert.Convert (which returns a Result for the
// caller to apply, rather than touching the backend itself).
func Apply(moves []PlannedMove, opts ApplyOptions) []ApplyOutcome {
	outcomes := make([]ApplyOutcome, 0, len(moves))

	for _, m := range moves {
		outcome := ApplyOutcome{BookID: m.BookID}

		if m.Skipped || m.Failed || m.Collision {
			outcome.Skipped = true
			outcomes = append(outcomes, outcome)
			continue
		}

		book, ok := opts.Books[m.BookID]
		if !ok {
			outcome.Failed = true
			outcome.FailReason = "book not found"
			outcomes = append(outcomes, outcome)
			continue
		}

		if m.NewResolvedPath == m.OldResolvedPath {
			outcome.NoOp = true
			advance(book, opts.Rulesets)
			outcomes = append(outcomes, outcome)
			continue
		}

		if m.OldRawPath == "" {
			// Fileless placeholder (comic-server-3bz.4's own
			// FilelessFormat case) - there is no real file to move or
			// copy, only the library record's own path field changes.
			book.FilePath = m.NewRawPath
			advance(book, opts.Rulesets)
			outcome.NoOp = true
			outcomes = append(outcomes, outcome)
			continue
		}

		if err := applyOne(m, opts); err != nil {
			outcome.Failed = true
			outcome.FailReason = err.Error()
			outcomes = append(outcomes, outcome)
			continue
		}

		book.FilePath = m.NewRawPath
		advance(book, opts.Rulesets)
		outcome.Applied = true
		outcomes = append(outcomes, outcome)
	}

	return outcomes
}

func advance(book *library.ComicBook, rulesets []datamanager.Ruleset) {
	workflow.AdvanceIfAtOrBefore(book, workflow.StageToMove, rulesets)
}

// applyOne performs the actual file operation for a single approved move.
func applyOne(m PlannedMove, opts ApplyOptions) error {
	if err := os.MkdirAll(filepath.Dir(m.NewResolvedPath), 0o755); err != nil {
		return fmt.Errorf("libraryorganizer: create destination dir: %w", err)
	}

	write := func(tmpPath string) error { return copyFile(m.OldResolvedPath, tmpPath) }
	validate := func(tmpPath string) error { return validateCopy(m.OldResolvedPath, tmpPath) }

	switch opts.Mode {
	case ModeCopy:
		if err := opts.Trash.WriteNew(m.NewResolvedPath, write, validate); err != nil {
			return fmt.Errorf("libraryorganizer: copy to %s: %w", m.NewResolvedPath, err)
		}
	default: // ModeMove
		if err := opts.Trash.WriteNew(m.NewResolvedPath, write, validate); err != nil {
			return fmt.Errorf("libraryorganizer: write %s: %w", m.NewResolvedPath, err)
		}
		if err := opts.Trash.Quarantine(m.OldResolvedPath); err != nil {
			os.Remove(m.NewResolvedPath) // avoid leaving an orphan new file behind
			return fmt.Errorf("libraryorganizer: quarantine original %s: %w", m.OldResolvedPath, err)
		}
	}

	return nil
}

// copyFile copies src's bytes to tmpPath - the write callback trash.WriteNew
// expects, adapted for "relocate existing bytes" rather than "generate new
// content" (see internal/trash's own doc comment on this distinction).
func copyFile(src, tmpPath string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// validateCopy sanity-checks a just-written copy before internal/trash is
// allowed to let it replace or occupy anything: its size must match the
// source's. Library Organizer moves arbitrary comic archive formats
// as-is (no re-encoding), so a byte-for-byte size match is the only
// generic corruption check that applies to every format - unlike
// cbzconvert's validateCBZ, which can inspect zip structure because it
// controls the output format.
func validateCopy(src, tmpPath string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	tmpInfo, err := os.Stat(tmpPath)
	if err != nil {
		return fmt.Errorf("stat written copy: %w", err)
	}
	if srcInfo.Size() != tmpInfo.Size() {
		return fmt.Errorf("written copy size %d does not match source size %d", tmpInfo.Size(), srcInfo.Size())
	}
	return nil
}
