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
func (t *Trash) Replace(targetPath string, write func(tmpPath string) error, validate func(tmpPath string) error) error {
	dir := filepath.Dir(targetPath)
	tmpPath, err := tempPathIn(dir, filepath.Base(targetPath))
	if err != nil {
		return fmt.Errorf("trash: create temp path: %w", err)
	}
	tmpConsumed := false
	defer func() {
		if !tmpConsumed {
			os.Remove(tmpPath)
		}
	}()

	if err := write(tmpPath); err != nil {
		return fmt.Errorf("trash: write new content: %w", err)
	}

	if validate != nil {
		if err := validate(tmpPath); err != nil {
			return fmt.Errorf("trash: validate new content: %w", err)
		}
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
	tmpConsumed = true

	return nil
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
