// Watch/reimport change detection (comic-server-zw0o, spec §5). Detects
// whether a single tracked CBL file changed between the commit it was
// imported at and the clone's current HEAD, using the real `git diff`
// (rename detection included) rather than a pure-Go re-implementation.
package cblrepo

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PathChangeStatus classifies what happened to a tracked CBL path
// between two commits.
type PathChangeStatus int

const (
	// PathUnchanged means the file at path is byte-identical to what it
	// was at oldSHA (no diff line touches it).
	PathUnchanged PathChangeStatus = iota
	// PathModified means the file still lives at the same path but its
	// content changed.
	PathModified
	// PathRenamed means git's own rename detection (`diff -M`) matched
	// the old path to a new one - NewPath is set. Content may or may not
	// have also changed; either way this is not treated as ambiguous,
	// since git already resolved it to a single candidate.
	PathRenamed
	// PathOrphaned means the path was deleted upstream and git did NOT
	// resolve it to a rename (either a genuine deletion, or a rewrite
	// too large for git's similarity heuristic to call a rename). Per
	// spec §5, this is deliberately never auto-resolved to a guessed
	// successor file - the caller should flag the list for manual
	// attention rather than silently dropping or reimporting it from
	// the wrong source.
	PathOrphaned
	// PathBaseUnknown means oldSHA no longer exists in the clone's
	// history (force-push or rebase upstream) - see Sync's fast-forward
	// -only fetch, which means a rewritten history simply won't contain
	// old commits at all. Diffing is impossible; the caller needs a
	// fresh import rather than a reimport (spec §5's documented
	// content-hash fallback is not implemented - see doc comment on
	// CheckPathChange).
	PathBaseUnknown
)

func (s PathChangeStatus) String() string {
	switch s {
	case PathUnchanged:
		return "unchanged"
	case PathModified:
		return "modified"
	case PathRenamed:
		return "renamed"
	case PathOrphaned:
		return "orphaned"
	case PathBaseUnknown:
		return "base_unknown"
	default:
		return "unknown"
	}
}

// PathChange is the result of CheckPathChange.
type PathChange struct {
	Status  PathChangeStatus
	NewPath string // set only when Status == PathRenamed
}

// HasCommit reports whether sha exists in the clone's history.
func (r *Repo) HasCommit(ctx context.Context, sha string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "cat-file", "-e", sha+"^{commit}")
	cmd.Dir = r.ClonePath
	if err := cmd.Run(); err != nil {
		// cat-file -e exits non-zero for "object doesn't exist", which
		// is the expected negative case, not a real error - only
		// something like a missing git binary or a non-repo ClonePath
		// would be a genuine failure, and both are already covered by
		// callers checking GitAvailable/Sync first.
		return false, nil
	}
	return true, nil
}

// CheckPathChange reports what happened to path between oldSHA and the
// clone's current HEAD.
//
// NOTE on the spec's documented content-hash fallback for a missing
// oldSHA: not implemented here. comic-server never commits into its own
// clone (Sync is fast-forward-only), so a missing oldSHA means the
// upstream remote rewrote history - rare for a public reading-list repo.
// Rather than compare file bytes against nothing (there is no stored
// original content to hash against, only the parsed CBLImportEntry rows
// - see storage.CBLImportSource doc comment), CheckPathChange reports
// PathBaseUnknown and leaves the caller to prompt for a fresh import.
// Revisit if this actually fires in practice.
func (r *Repo) CheckPathChange(ctx context.Context, oldSHA, path string) (PathChange, error) {
	if oldSHA == "" {
		return PathChange{}, fmt.Errorf("cblrepo: oldSHA is required")
	}
	if ok, err := r.HasCommit(ctx, oldSHA); err != nil {
		return PathChange{}, err
	} else if !ok {
		return PathChange{Status: PathBaseUnknown}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	// Full-repo diff, not `-- path` scoped: git only attributes a rename
	// to a pathspec-matched pair when BOTH sides match the pathspec,
	// which would hide a rename whose new name doesn't match `path`.
	// The repo is small (a few thousand files) so this is cheap.
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-status", "-M", oldSHA, "HEAD")
	cmd.Dir = r.ClonePath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return PathChange{}, fmt.Errorf("git diff --name-status -M %s HEAD: %w: %s", oldSHA, err, strings.TrimSpace(stderr.String()))
	}

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 2 {
			continue
		}
		status, rest := fields[0], fields[1:]
		switch {
		case strings.HasPrefix(status, "R"):
			if len(rest) < 2 {
				continue
			}
			oldPath, newPath := rest[0], rest[1]
			if oldPath == path {
				return PathChange{Status: PathRenamed, NewPath: newPath}, nil
			}
		case status == "M":
			if rest[0] == path {
				return PathChange{Status: PathModified}, nil
			}
		case status == "D":
			if rest[0] == path {
				return PathChange{Status: PathOrphaned}, nil
			}
		// "A" (added) never applies to an existing tracked path.
		default:
		}
	}
	if err := scanner.Err(); err != nil {
		return PathChange{}, fmt.Errorf("scan git diff output: %w", err)
	}
	return PathChange{Status: PathUnchanged}, nil
}
