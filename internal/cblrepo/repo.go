// Package cblrepo clones and browses a git-hosted collection of CBL
// reading lists (comic-server-oprf, spec
// docs/plans/2026-09-20-cbl-reading-list-import.md §3), primarily
// DieselTech/CBL-ReadingLists. It shells out to a real `git` binary
// rather than using a pure-Go library - comic-server runs in Docker,
// which guarantees git is present, and this gives real rename detection
// and history-diffing for free for the future watch/reimport feature
// (comic-server-zw0o, session decision 2026-09-20).
//
// PERSONAL USE ONLY: the default repo has no license file and its own
// README disclaims ownership of its content (spec §3). This package
// clones for the local operator's own browsing/import - it must never
// redistribute or bundle the cloned content elsewhere.
package cblrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// gitTimeout bounds a single git subprocess (clone/fetch/pull) - the
// DieselTech repo is ~200MB; this is generous but not unbounded, so a
// hung network operation doesn't block a request indefinitely.
const gitTimeout = 5 * time.Minute

// ErrGitNotFound means the `git` binary isn't on PATH. Docker guarantees
// it; a non-Docker/local dev run might not have it - callers should
// disable this feature gracefully rather than fail startup (spec §5).
var ErrGitNotFound = errors.New("cblrepo: git binary not found on PATH")

// ErrClonePathNotAGitRepo means ClonePath exists, is non-empty, but isn't
// a git working tree - almost certainly a misconfiguration (pointed at
// the wrong directory) rather than something to silently paper over.
var ErrClonePathNotAGitRepo = errors.New("cblrepo: clone path exists and is not a git repository")

// Repo is a local clone of a CBL-hosting git repository, ready to browse
// and import from.
type Repo struct {
	URL       string
	ClonePath string
}

// New returns a Repo for url, cloned/maintained at clonePath.
func New(url, clonePath string) *Repo {
	return &Repo{URL: url, ClonePath: clonePath}
}

// GitAvailable reports whether the `git` binary is on PATH.
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Sync brings the local clone up to date: clones if ClonePath doesn't
// exist yet, otherwise fast-forwards it. A full (non-shallow) clone is
// used deliberately - shallow clones can't diff an old commit against a
// new HEAD, which the not-yet-built watch/reimport feature
// (comic-server-zw0o) will need; cloning shallow now would mean
// re-cloning later.
//
// On any git failure, if a usable clone ALREADY exists at ClonePath, Sync
// logs nothing itself (callers should) but returns nil rather than an
// error - spec §3's "tolerate an unreachable repo, use the last good
// clone" fallback. It only returns an error when there's nothing usable
// to fall back to.
func (r *Repo) Sync(ctx context.Context) error {
	if !GitAvailable() {
		return ErrGitNotFound
	}

	hasClone := r.isGitRepo()

	if !hasClone {
		if err := r.ensureEmptyOrMissing(); err != nil {
			return err
		}
		if err := r.runGit(ctx, "", "clone", r.URL, r.ClonePath); err != nil {
			return fmt.Errorf("clone %s: %w", r.URL, err)
		}
		return nil
	}

	// Already cloned - fast-forward only. We never commit into this
	// clone ourselves, so a non-fast-forward pull would mean something
	// unexpected happened to the local clone (manual edit, corruption) -
	// surface that rather than silently discarding it with --hard.
	if err := r.runGit(ctx, r.ClonePath, "pull", "--ff-only"); err != nil {
		// Fallback: an existing, previously-good clone stays usable even
		// if the network/remote is unreachable right now.
		return nil
	}
	return nil
}

func (r *Repo) isGitRepo() bool {
	info, err := os.Stat(filepath.Join(r.ClonePath, ".git"))
	return err == nil && info.IsDir()
}

func (r *Repo) ensureEmptyOrMissing() error {
	entries, err := os.ReadDir(r.ClonePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check clone path: %w", err)
	}
	if len(entries) > 0 {
		return ErrClonePathNotAGitRepo
	}
	return nil
}

func (r *Repo) runGit(ctx context.Context, dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// HeadCommit returns the clone's current commit SHA (git rev-parse
// HEAD), used as CBLImportSource.SourceRef for entries imported from
// this repo.
func (r *Repo) HeadCommit() (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = r.ClonePath
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ListCBLFiles walks the clone and returns every .cbl file's path
// relative to the clone root (forward-slash separated regardless of OS),
// sorted. The repo's own .git directory is always excluded.
func (r *Repo) ListCBLFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(r.ClonePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".cbl") {
			return nil
		}
		rel, err := filepath.Rel(r.ClonePath, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk clone: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

// Search filters ListCBLFiles by a case-insensitive substring match
// against each file's relative path (so a query matches on folder name,
// e.g. "Batman", as well as filename).
func (r *Repo) Search(query string) ([]string, error) {
	all, err := r.ListCBLFiles()
	if err != nil {
		return nil, err
	}
	if query == "" {
		return all, nil
	}
	q := strings.ToLower(query)
	var out []string
	for _, f := range all {
		if strings.Contains(strings.ToLower(f), q) {
			out = append(out, f)
		}
	}
	return out, nil
}

// ErrPathEscapesClone is returned by Open when relPath would resolve
// outside ClonePath - guards against a path-traversal relPath reaching a
// file outside the intended clone (defense in depth; callers should
// already only pass paths returned by ListCBLFiles/Search).
var ErrPathEscapesClone = errors.New("cblrepo: path escapes the clone directory")

// Open returns the contents of relPath within the clone. The caller must
// Close the returned file.
func (r *Repo) Open(relPath string) (*os.File, error) {
	full := filepath.Join(r.ClonePath, filepath.FromSlash(relPath))
	cleanRoot, err := filepath.Abs(r.ClonePath)
	if err != nil {
		return nil, err
	}
	cleanFull, err := filepath.Abs(full)
	if err != nil {
		return nil, err
	}
	if cleanFull != cleanRoot && !strings.HasPrefix(cleanFull, cleanRoot+string(filepath.Separator)) {
		return nil, ErrPathEscapesClone
	}
	return os.Open(cleanFull)
}
