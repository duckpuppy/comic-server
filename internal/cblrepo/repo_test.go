package cblrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newLocalGitFixture creates a real local git repository (not a clone of
// anything remote - a from-scratch `git init` + commit) with a handful
// of .cbl files in a folder structure, so Sync/ListCBLFiles/Search/Open
// can be exercised against a REAL git repo without any network
// dependency or the cost of a ~200MB DieselTech clone in every test run.
func newLocalGitFixture(t *testing.T) string {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()

	files := map[string]string{
		"DC/Batman/Batman No Events.cbl":  `<ReadingList><Name>Batman No Events</Name></ReadingList>`,
		"DC/Superman/Superman Origin.cbl": `<ReadingList><Name>Superman Origin</Name></ReadingList>`,
		"Marvel/X-Men/Dark Phoenix.cbl":   `<ReadingList><Name>Dark Phoenix</Name></ReadingList>`,
		"README.md":                       "not a CBL file",
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	runOrFatal := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	runOrFatal("init", "-q")
	runOrFatal("add", "-A")
	runOrFatal("commit", "-q", "-m", "initial")

	return dir
}

func TestSync_ClonesFromLocalRepo(t *testing.T) {
	source := newLocalGitFixture(t)
	clonePath := filepath.Join(t.TempDir(), "clone")

	r := New(source, clonePath)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync (initial clone): %v", err)
	}

	files, err := r.ListCBLFiles()
	if err != nil {
		t.Fatalf("ListCBLFiles: %v", err)
	}
	want := []string{"DC/Batman/Batman No Events.cbl", "DC/Superman/Superman Origin.cbl", "Marvel/X-Men/Dark Phoenix.cbl"}
	if len(files) != len(want) {
		t.Fatalf("ListCBLFiles = %v, want %v", files, want)
	}
	for i, w := range want {
		if files[i] != w {
			t.Errorf("files[%d] = %q, want %q", i, files[i], w)
		}
	}

	commit, err := r.HeadCommit()
	if err != nil || commit == "" {
		t.Fatalf("HeadCommit: commit=%q err=%v", commit, err)
	}
}

func TestSync_PullsUpdatesOnExistingClone(t *testing.T) {
	source := newLocalGitFixture(t)
	clonePath := filepath.Join(t.TempDir(), "clone")

	r := New(source, clonePath)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync (initial clone): %v", err)
	}
	before, _ := r.ListCBLFiles()
	if len(before) != 3 {
		t.Fatalf("expected 3 files before update, got %d", len(before))
	}

	// Add a new file upstream and commit.
	newFile := filepath.Join(source, "Marvel", "Spider-Man", "Amazing.cbl")
	if err := os.MkdirAll(filepath.Dir(newFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("<ReadingList/>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "add spider-man"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = source
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync (pull update): %v", err)
	}
	after, err := r.ListCBLFiles()
	if err != nil {
		t.Fatalf("ListCBLFiles after update: %v", err)
	}
	if len(after) != 4 {
		t.Fatalf("expected 4 files after update, got %d: %v", len(after), after)
	}
}

func TestSync_UnreachableRemoteFallsBackToExistingClone(t *testing.T) {
	source := newLocalGitFixture(t)
	clonePath := filepath.Join(t.TempDir(), "clone")

	r := New(source, clonePath)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync (initial clone): %v", err)
	}

	// Point the clone's origin at a URL that will never resolve, then
	// Sync again - the existing clone must remain usable rather than
	// Sync erroring the whole operation (spec §3 fallback).
	cmd := exec.Command("git", "remote", "set-url", "origin", "https://cbl-repo-does-not-exist.invalid/nope.git")
	cmd.Dir = clonePath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote set-url: %v: %s", err, out)
	}

	r2 := New("https://cbl-repo-does-not-exist.invalid/nope.git", clonePath)
	if err := r2.Sync(context.Background()); err != nil {
		t.Errorf("Sync with unreachable remote on an existing clone should fall back, not error: %v", err)
	}
	files, err := r2.ListCBLFiles()
	if err != nil || len(files) != 3 {
		t.Errorf("expected the pre-existing 3 files to remain usable after a failed sync, got files=%v err=%v", files, err)
	}
}

func TestSearch_FiltersByPathSubstring(t *testing.T) {
	source := newLocalGitFixture(t)
	clonePath := filepath.Join(t.TempDir(), "clone")
	r := New(source, clonePath)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	got, err := r.Search("batman")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0] != "DC/Batman/Batman No Events.cbl" {
		t.Errorf("Search(batman) = %v, want [DC/Batman/Batman No Events.cbl]", got)
	}

	got, err = r.Search("dc/")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Search(dc/) = %v, want 2 results", got)
	}

	got, err = r.Search("")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Search(\"\") = %v, want all 3 files", got)
	}
}

func TestOpen_RejectsPathTraversal(t *testing.T) {
	source := newLocalGitFixture(t)
	clonePath := filepath.Join(t.TempDir(), "clone")
	r := New(source, clonePath)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	f, err := r.Open("DC/Batman/Batman No Events.cbl")
	if err != nil {
		t.Fatalf("Open valid path: %v", err)
	}
	f.Close()

	for _, bad := range []string{"../../../etc/passwd", "../outside.cbl", "DC/../../outside.cbl"} {
		if _, err := r.Open(bad); err != ErrPathEscapesClone {
			t.Errorf("Open(%q): err = %v, want ErrPathEscapesClone", bad, err)
		}
	}
}

func TestSync_RejectsNonEmptyNonGitClonePath(t *testing.T) {
	clonePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(clonePath, "somefile.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed non-git dir: %v", err)
	}

	r := New("https://example.invalid/repo.git", clonePath)
	if err := r.Sync(context.Background()); err != ErrClonePathNotAGitRepo {
		t.Errorf("err = %v, want ErrClonePathNotAGitRepo", err)
	}
}
