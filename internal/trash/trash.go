// Package trash implements comic-server's safe file-replace mechanism:
// write-new-to-temp-then-atomic-rename, plus quarantining (not deleting)
// the file being replaced. It exists because comic-server's first
// archive-writing feature (CBZ conversion, comic-server-43b) would
// otherwise be the first thing in the codebase that deletes a user's comic
// file - on Linux, unlike Windows' Recycle Bin, a bare os.Remove is not
// recoverable. See comic-server-1up for the full design record and
// comic-server-rhe for this package's own issue.
//
// This package is deliberately format-agnostic: it knows nothing about
// zip/CBZ/ComicInfo.xml. Callers supply a write func (produce the new file
// content) and an optional validate func (sanity-check it before it's
// allowed to replace the original).
package trash

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// trashSuffixSep separates a quarantined file's original name from its
// trashed-at timestamp, e.g. "Bar.cbr~1735689600". Sweep parses this back
// out rather than relying on filesystem mtime, which os.Rename generally
// preserves from the original file - an mtime-based sweep would delete
// long-owned files the moment they're quarantined.
const trashSuffixSep = "~"

// Trash quarantines replaced files under Root and can later sweep entries
// older than RetentionDays. Safe for concurrent use.
type Trash struct {
	// Root is the quarantine directory. Files are moved here (mirroring
	// their original absolute path, so multiple replaced files never
	// collide by basename alone) rather than deleted. Must be on a
	// filesystem the process can write to; if it's on a different
	// filesystem than the files being replaced, Replace falls back to
	// copy+delete for the quarantine move (the final swap into the
	// original's place is always same-filesystem and stays a true atomic
	// rename regardless).
	Root string

	// RetentionDays is how long a quarantined file is kept before Sweep
	// removes it for good. Per comic-server-1up, comic-server always
	// auto-purges - callers needing an unbounded retention are expected to
	// keep RetentionDays generous rather than disabling sweeps.
	RetentionDays int
}

// New returns a Trash. retentionDays must be positive; callers should
// apply config.DefaultServerConfig's default (30) before calling this,
// same as other server settings.
func New(root string, retentionDays int) (*Trash, error) {
	if root == "" {
		return nil, errors.New("trash: root path is required")
	}
	if retentionDays <= 0 {
		return nil, fmt.Errorf("trash: retentionDays must be positive, got %d", retentionDays)
	}
	return &Trash{Root: root, RetentionDays: retentionDays}, nil
}

// Replace safely overwrites targetPath with new content:
//  1. write is called with a temp file path in targetPath's own directory
//     (guaranteeing the final swap is same-filesystem, hence atomic).
//  2. If validate is non-nil, it's called on the temp file before anything
//     about the original is touched. A validate error leaves targetPath
//     completely untouched and cleans up the temp file.
//  3. The ORIGINAL file is moved into quarantine (not deleted).
//  4. The temp file is renamed into targetPath's place.
//
// If step 4 fails after step 3 succeeded, Replace makes a best-effort
// attempt to move the quarantined original back before returning the
// error, so a mid-operation failure never leaves targetPath missing.
//
// Replace assumes the new content belongs at the SAME path as the
// original (e.g. rewriting a file's contents in place). A caller that
// writes its replacement to a DIFFERENT path - e.g. a format conversion
// that changes the file extension, where the original and the new file
// are never actually the same path - can't use a single-path swap; use
// WriteNew for the new file plus a separate Quarantine call for the old
// one instead. See comic-server-43b, the feature that surfaced this.
func (t *Trash) Replace(targetPath string, write func(tmpPath string) error, validate func(tmpPath string) error) error {
	tmpPath, tmpConsumed, err := t.writeValidated(targetPath, write, validate)
	defer func() {
		if !*tmpConsumed {
			os.Remove(tmpPath)
		}
	}()
	if err != nil {
		return err
	}

	quarantinePath := t.quarantinePathFor(targetPath, time.Now())
	if err := os.MkdirAll(filepath.Dir(quarantinePath), 0o755); err != nil {
		return fmt.Errorf("trash: create quarantine dir: %w", err)
	}
	if err := moveFile(targetPath, quarantinePath); err != nil {
		return fmt.Errorf("trash: quarantine original: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		// Best-effort rollback: put the original back so targetPath isn't
		// left missing. If this also fails, the original is still safely
		// sitting in quarantine, just not restored automatically.
		if rbErr := moveFile(quarantinePath, targetPath); rbErr != nil {
			return fmt.Errorf("trash: swap in new content: %w (rollback also failed: %v; original is at %s)", err, rbErr, quarantinePath)
		}
		return fmt.Errorf("trash: swap in new content: %w (rolled back, original restored)", err)
	}
	*tmpConsumed = true

	return nil
}

// WriteNew atomically creates a brand-new file at targetPath - crash-safe
// (temp file + atomic rename), same as Replace's write/validate/swap
// mechanics, but with no quarantine step, since there's nothing at
// targetPath to protect. Returns an error if targetPath already exists,
// so callers never silently clobber an existing file this way - use
// Replace instead when overwriting an existing file at the same path is
// the intent.
func (t *Trash) WriteNew(targetPath string, write func(tmpPath string) error, validate func(tmpPath string) error) error {
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("trash: WriteNew: %s already exists", targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("trash: WriteNew: stat %s: %w", targetPath, err)
	}

	tmpPath, tmpConsumed, err := t.writeValidated(targetPath, write, validate)
	defer func() {
		if !*tmpConsumed {
			os.Remove(tmpPath)
		}
	}()
	if err != nil {
		return err
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("trash: create new file: %w", err)
	}
	*tmpConsumed = true

	return nil
}

// Quarantine moves an existing file into the quarantine directory (not
// deleting it) without writing any replacement - for callers pairing it
// with WriteNew at a different path (see Replace's doc comment for why
// that split exists). Sweep/RetentionDays apply the same as any file
// quarantined via Replace.
func (t *Trash) Quarantine(path string) error {
	quarantinePath := t.quarantinePathFor(path, time.Now())
	if err := os.MkdirAll(filepath.Dir(quarantinePath), 0o755); err != nil {
		return fmt.Errorf("trash: create quarantine dir: %w", err)
	}
	if err := moveFile(path, quarantinePath); err != nil {
		return fmt.Errorf("trash: quarantine: %w", err)
	}
	return nil
}

// writeValidated writes new content to a temp file in targetPath's own
// directory (guaranteeing a later same-filesystem atomic rename) and runs
// validate on it, returning the temp path ready to be renamed into place.
// The returned consumed flag starts false; the caller must defer removing
// tmpPath unless consumed, and set *consumed = true once it successfully
// renames tmpPath away (a plain closure captured by defer at this point
// would freeze the pre-rename function value, not observe a later
// reassignment - the pointer is what lets the caller flip it afterward).
func (t *Trash) writeValidated(targetPath string, write func(tmpPath string) error, validate func(tmpPath string) error) (tmpPath string, consumed *bool, err error) {
	consumed = new(bool)
	dir := filepath.Dir(targetPath)
	tmpPath, err = tempPathIn(dir, filepath.Base(targetPath))
	if err != nil {
		return "", consumed, fmt.Errorf("trash: create temp path: %w", err)
	}

	if err := write(tmpPath); err != nil {
		return tmpPath, consumed, fmt.Errorf("trash: write new content: %w", err)
	}

	if validate != nil {
		if err := validate(tmpPath); err != nil {
			return tmpPath, consumed, fmt.Errorf("trash: validate new content: %w", err)
		}
	}

	return tmpPath, consumed, nil
}

// quarantinePathFor computes where targetPath's original content is
// quarantined to: its absolute path (minus any leading separator/volume)
// mirrored under Root, with the trashed-at timestamp appended to the
// basename so repeated replacements of the same path never collide.
func (t *Trash) quarantinePathFor(targetPath string, at time.Time) string {
	abs := targetPath
	if a, err := filepath.Abs(targetPath); err == nil {
		abs = a
	}
	abs = filepath.ToSlash(abs)
	abs = strings.TrimPrefix(abs, "/")
	// Strip a Windows-style drive letter ("C:") if present, since it's not
	// a valid path component to nest a directory under.
	if len(abs) >= 2 && abs[1] == ':' {
		abs = abs[2:]
		abs = strings.TrimPrefix(abs, "/")
	}
	mirrored := filepath.FromSlash(abs)
	dir := filepath.Dir(mirrored)
	base := filepath.Base(mirrored) + trashSuffixSep + strconv.FormatInt(at.Unix(), 10)
	return filepath.Join(t.Root, dir, base)
}

// Entry describes one quarantined file, for List/Restore callers (e.g. the
// web UI's trash browser, comic-server-tfs).
type Entry struct {
	// ID identifies this entry for a later Restore call: its path relative
	// to Root, using forward slashes regardless of OS (so it round-trips
	// safely through a URL/JSON API). Not guaranteed stable across a
	// restore-then-requarantine of the same original file, since a new
	// quarantine gets a new trashed-at timestamp.
	ID string

	// OriginalPath is the absolute path this file was quarantined from,
	// recovered from ID the same way quarantinePathFor derived it.
	OriginalPath string

	// QuarantinedAt is when this file was moved into quarantine, parsed
	// from its "~<unixSeconds>" suffix - the same source Sweep uses to
	// decide what's old enough to purge.
	QuarantinedAt time.Time

	// Size is the quarantined file's size in bytes.
	Size int64
}

// List returns every quarantined file under Root, newest first. Entries
// whose name doesn't match the "name~unixSeconds" quarantine format are
// skipped, same as Sweep.
func (t *Trash) List() ([]Entry, error) {
	var entries []Entry
	walkErr := filepath.WalkDir(t.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == t.Root {
				return filepath.SkipDir // trash root doesn't exist yet - nothing to list
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		trashedAt, ok := parseTrashedAt(d.Name())
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		id, err := filepath.Rel(t.Root, path)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{
			ID:            filepath.ToSlash(id),
			OriginalPath:  t.originalPathFor(id),
			QuarantinedAt: trashedAt,
			Size:          info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("trash: list: %w", walkErr)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].QuarantinedAt.After(entries[j].QuarantinedAt)
	})
	return entries, nil
}

// Restore moves a quarantined entry (by the ID returned from List) back to
// its original path. If that path is currently occupied by a different
// file (e.g. the CBZ a book was converted to), the occupant is quarantined
// first - a fresh Entry, restorable the same way - before the original is
// moved back, so a restore is itself always undoable and nothing already
// on disk is ever deleted as a side effect of restoring something else.
func (t *Trash) Restore(id string) error {
	quarantinePath, err := t.resolveID(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(quarantinePath); err != nil {
		return fmt.Errorf("trash: restore: %w", err)
	}

	originalPath := t.originalPathFor(id)

	if _, err := os.Stat(originalPath); err == nil {
		// Something's occupying the original path - quarantine it first
		// (same two steps Quarantine itself performs) so restoring never
		// clobbers or deletes it. quarantinePathFor only has second
		// resolution, so within a fast restore-then-requarantine sequence
		// (or just bad luck) "now" can collide with the entry being
		// restored's own timestamp - walk the clock forward a second at a
		// time until the path is actually free.
		now := time.Now()
		occupantQuarantinePath := t.quarantinePathFor(originalPath, now)
		for n := int64(1); occupantQuarantinePath == quarantinePath; n++ {
			occupantQuarantinePath = t.quarantinePathFor(originalPath, now.Add(time.Duration(n)*time.Second))
		}
		if err := os.MkdirAll(filepath.Dir(occupantQuarantinePath), 0o755); err != nil {
			return fmt.Errorf("trash: restore: quarantine current occupant: %w", err)
		}
		if err := moveFile(originalPath, occupantQuarantinePath); err != nil {
			return fmt.Errorf("trash: restore: quarantine current occupant: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("trash: restore: stat original path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		return fmt.Errorf("trash: restore: %w", err)
	}
	if err := moveFile(quarantinePath, originalPath); err != nil {
		return fmt.Errorf("trash: restore: %w", err)
	}
	return nil
}

// resolveID converts an Entry.ID back to an absolute quarantine path,
// rejecting anything that would resolve outside Root (a malformed or
// tampered-with ID from an API caller).
func (t *Trash) resolveID(id string) (string, error) {
	if id == "" || strings.Contains(id, "..") {
		return "", fmt.Errorf("trash: invalid entry id %q", id)
	}
	full := filepath.Join(t.Root, filepath.FromSlash(id))
	rel, err := filepath.Rel(t.Root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("trash: invalid entry id %q", id)
	}
	return full, nil
}

// originalPathFor recovers the absolute path an entry (by ID, relative to
// Root) was quarantined from - the inverse of quarantinePathFor: strip the
// trailing "~<unixSeconds>" suffix from the basename, then treat the rest
// as an absolute path rooted at "/" (quarantinePathFor stripped the
// leading separator and any Windows drive letter when it built this same
// relative path).
func (t *Trash) originalPathFor(id string) string {
	slashID := filepath.ToSlash(id)
	idx := strings.LastIndex(slashID, trashSuffixSep)
	if idx != -1 {
		slashID = slashID[:idx]
	}
	return filepath.FromSlash("/" + slashID)
}

// SweepResult reports the outcome of one Sweep pass.
type SweepResult struct {
	Removed int
	Errs    []error
}

// Sweep deletes quarantined files trashed more than RetentionDays ago
// (relative to now), then prunes any directories under Root left empty by
// those deletions. Entries whose name doesn't match the expected
// "name~unixSeconds" quarantine format are left alone - Sweep only ever
// removes files it recognizes as its own.
func (t *Trash) Sweep(now time.Time) SweepResult {
	var result SweepResult
	cutoff := now.Add(-time.Duration(t.RetentionDays) * 24 * time.Hour)

	var dirs []string
	walkErr := filepath.WalkDir(t.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == t.Root {
				return filepath.SkipDir // trash root doesn't exist yet - nothing to sweep
			}
			result.Errs = append(result.Errs, err)
			return nil
		}
		if d.IsDir() {
			if path != t.Root {
				dirs = append(dirs, path)
			}
			return nil
		}
		trashedAt, ok := parseTrashedAt(d.Name())
		if !ok {
			return nil
		}
		if trashedAt.After(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			result.Errs = append(result.Errs, err)
			return nil
		}
		result.Removed++
		return nil
	})
	if walkErr != nil {
		result.Errs = append(result.Errs, walkErr)
	}

	// Prune now-empty directories, deepest first, best-effort.
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Remove(dirs[i]) // no-op (and ignored) if not actually empty
	}

	return result
}

// parseTrashedAt extracts the unix-seconds timestamp from a quarantined
// file's basename, per quarantinePathFor's "name~unixSeconds" format.
func parseTrashedAt(name string) (time.Time, bool) {
	idx := strings.LastIndex(name, trashSuffixSep)
	if idx == -1 || idx == len(name)-1 {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(name[idx+1:], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// tempPathIn returns an unused path in dir for a temp file related to
// baseName, using a random suffix so concurrent Replace calls on
// differently-named files never collide (same-named concurrent calls are
// still the caller's responsibility to serialize, same as any other
// filesystem operation on a single logical resource).
func tempPathIn(dir, baseName string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	name := "." + baseName + ".tmp-" + hex.EncodeToString(suffix[:])
	return filepath.Join(dir, name), nil
}

// moveFile moves src to dst, trying an atomic rename first and falling
// back to copy+remove if src/dst are on different filesystems (os.Rename
// returns a LinkError wrapping syscall.EXDEV in that case on Linux).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyThenRemove(src, dst)
}

func copyThenRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}
