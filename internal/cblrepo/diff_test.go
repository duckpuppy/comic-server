package cblrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepoFixture is like newLocalGitFixture (repo_test.go) but returns a
// helper to make further commits, since diff tests need at least two
// commits to diff between.
type gitRepoFixture struct {
	dir string
	t   *testing.T
}

func newGitRepoFixture(t *testing.T) *gitRepoFixture {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	f := &gitRepoFixture{dir: dir, t: t}
	f.run("init", "-q")
	return f
}

func (f *gitRepoFixture) run(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *gitRepoFixture) write(rel, content string) {
	f.t.Helper()
	full := filepath.Join(f.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatalf("write %s: %v", rel, err)
	}
}

func (f *gitRepoFixture) remove(rel string) {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.dir, filepath.FromSlash(rel))); err != nil {
		f.t.Fatalf("remove %s: %v", rel, err)
	}
}

func (f *gitRepoFixture) commit(msg string) string {
	f.t.Helper()
	f.run("add", "-A")
	f.run("commit", "-q", "-m", msg)
	return f.run("rev-parse", "HEAD")
}

func (f *gitRepoFixture) repo() *Repo {
	return &Repo{ClonePath: f.dir}
}

func TestCheckPathChange_Unchanged(t *testing.T) {
	f := newGitRepoFixture(t)
	f.write("DC/Batman/Batman.cbl", "<ReadingList><Name>Batman</Name></ReadingList>")
	base := f.commit("initial")
	// A commit that touches an unrelated file - our tracked path is
	// untouched by it.
	f.write("Marvel/XMen.cbl", "<ReadingList><Name>X-Men</Name></ReadingList>")
	f.commit("add unrelated file")

	change, err := f.repo().CheckPathChange(context.Background(), base, "DC/Batman/Batman.cbl")
	if err != nil {
		t.Fatalf("CheckPathChange: %v", err)
	}
	if change.Status != PathUnchanged {
		t.Errorf("Status = %v, want PathUnchanged", change.Status)
	}
}

func TestCheckPathChange_Modified(t *testing.T) {
	f := newGitRepoFixture(t)
	f.write("DC/Batman/Batman.cbl", "<ReadingList><Name>Batman</Name></ReadingList>")
	base := f.commit("initial")
	f.write("DC/Batman/Batman.cbl", "<ReadingList><Name>Batman v2</Name><Books><Book Series=\"Batman\" Number=\"1\"/></Books></ReadingList>")
	f.commit("update batman list")

	change, err := f.repo().CheckPathChange(context.Background(), base, "DC/Batman/Batman.cbl")
	if err != nil {
		t.Fatalf("CheckPathChange: %v", err)
	}
	if change.Status != PathModified {
		t.Errorf("Status = %v, want PathModified", change.Status)
	}
}

func TestCheckPathChange_Renamed(t *testing.T) {
	f := newGitRepoFixture(t)
	content := "<ReadingList><Name>Batman</Name><Books>" +
		strings.Repeat(`<Book Series="Batman" Number="1"/>`, 20) +
		"</Books></ReadingList>"
	f.write("DC/Batman/Batman.cbl", content)
	base := f.commit("initial")
	f.remove("DC/Batman/Batman.cbl")
	f.write("DC/Batman/Batman Classic.cbl", content) // identical content = high similarity
	f.commit("rename batman list")

	change, err := f.repo().CheckPathChange(context.Background(), base, "DC/Batman/Batman.cbl")
	if err != nil {
		t.Fatalf("CheckPathChange: %v", err)
	}
	if change.Status != PathRenamed {
		t.Fatalf("Status = %v, want PathRenamed", change.Status)
	}
	if change.NewPath != "DC/Batman/Batman Classic.cbl" {
		t.Errorf("NewPath = %q, want %q", change.NewPath, "DC/Batman/Batman Classic.cbl")
	}
}

func TestCheckPathChange_Orphaned(t *testing.T) {
	f := newGitRepoFixture(t)
	f.write("DC/Batman/Batman.cbl", "<ReadingList><Name>Batman</Name></ReadingList>")
	f.write("keep.txt", "unrelated")
	base := f.commit("initial")
	f.remove("DC/Batman/Batman.cbl")
	// No similarly-shaped replacement file - git has nothing to call a
	// rename, so this should report Orphaned, not guess a successor.
	f.commit("delete batman list")

	change, err := f.repo().CheckPathChange(context.Background(), base, "DC/Batman/Batman.cbl")
	if err != nil {
		t.Fatalf("CheckPathChange: %v", err)
	}
	if change.Status != PathOrphaned {
		t.Errorf("Status = %v, want PathOrphaned", change.Status)
	}
}

func TestCheckPathChange_BaseUnknown(t *testing.T) {
	f := newGitRepoFixture(t)
	f.write("DC/Batman/Batman.cbl", "<ReadingList><Name>Batman</Name></ReadingList>")
	f.commit("initial")

	change, err := f.repo().CheckPathChange(context.Background(), "0000000000000000000000000000000000dead", "DC/Batman/Batman.cbl")
	if err != nil {
		t.Fatalf("CheckPathChange: %v", err)
	}
	if change.Status != PathBaseUnknown {
		t.Errorf("Status = %v, want PathBaseUnknown", change.Status)
	}
}

func TestHasCommit(t *testing.T) {
	f := newGitRepoFixture(t)
	f.write("a.cbl", "x")
	sha := f.commit("initial")

	ok, err := f.repo().HasCommit(context.Background(), sha)
	if err != nil || !ok {
		t.Errorf("HasCommit(%s) = %v, %v, want true, nil", sha, ok, err)
	}
	ok, err = f.repo().HasCommit(context.Background(), "0000000000000000000000000000000000dead")
	if err != nil || ok {
		t.Errorf("HasCommit(bogus) = %v, %v, want false, nil", ok, err)
	}
}
