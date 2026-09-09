package libraryorganizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

func newTestTrash(t *testing.T) *trash.Trash {
	t.Helper()
	root := t.TempDir()
	tr, err := trash.New(root, 30)
	if err != nil {
		t.Fatalf("trash.New: %v", err)
	}
	return tr
}

func writeSourceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestApply_MoveWritesDestAndQuarantinesSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.cbz")
	dst := filepath.Join(dir, "new", "Sandman 01.cbz")
	writeSourceFile(t, src, "comic bytes")

	book := &library.ComicBook{ID: "b1", FilePath: `old.cbz`}
	moves := []PlannedMove{{
		BookID: "b1", OldRawPath: `old.cbz`, NewRawPath: `new/Sandman 01.cbz`,
		OldResolvedPath: src, NewResolvedPath: dst,
	}}

	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeMove,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{"b1": book},
	})

	if len(outcomes) != 1 || !outcomes[0].Applied {
		t.Fatalf("outcomes = %+v, want one Applied", outcomes)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected destination file to exist: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected source to be gone (quarantined), stat err = %v", err)
	}
	if book.FilePath != `new/Sandman 01.cbz` {
		t.Errorf("book.FilePath = %q, want updated to NewRawPath", book.FilePath)
	}
	if got := workflow.GetStage(book); got != workflow.StageOrganized {
		t.Errorf("GetStage = %v, want StageOrganized", got)
	}
}

func TestApply_CopyLeavesSourceInPlace(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.cbz")
	dst := filepath.Join(dir, "new", "copy.cbz")
	writeSourceFile(t, src, "comic bytes")

	book := &library.ComicBook{ID: "b1", FilePath: `old.cbz`}
	moves := []PlannedMove{{
		BookID: "b1", OldRawPath: `old.cbz`, NewRawPath: `new/copy.cbz`,
		OldResolvedPath: src, NewResolvedPath: dst,
	}}

	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeCopy,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{"b1": book},
	})

	if len(outcomes) != 1 || !outcomes[0].Applied {
		t.Fatalf("outcomes = %+v, want one Applied", outcomes)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected destination file to exist: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("expected source to still exist (Copy mode): %v", err)
	}
}

func TestApply_SkippedFailedCollisionNeverTouchDisk(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.cbz")
	writeSourceFile(t, src, "comic bytes")

	book := &library.ComicBook{ID: "b1", FilePath: `old.cbz`}
	moves := []PlannedMove{
		{BookID: "b1", OldResolvedPath: src, Skipped: true},
		{BookID: "b1", OldResolvedPath: src, Failed: true},
		{BookID: "b1", OldResolvedPath: src, Collision: true},
	}

	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeMove,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{"b1": book},
	})

	for i, o := range outcomes {
		if !o.Skipped || o.Applied {
			t.Errorf("moves[%d] outcome = %+v, want Skipped only", i, o)
		}
	}
	if book.FilePath != `old.cbz` {
		t.Errorf("book.FilePath changed to %q, want untouched", book.FilePath)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file should be untouched: %v", err)
	}
}

func TestApply_NoOpAlreadyAtDestinationStillAdvancesStage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "already-there.cbz")
	writeSourceFile(t, path, "comic bytes")

	book := &library.ComicBook{ID: "b1", FilePath: `already-there.cbz`}
	moves := []PlannedMove{{
		BookID: "b1", OldRawPath: `already-there.cbz`, NewRawPath: `already-there.cbz`,
		OldResolvedPath: path, NewResolvedPath: path,
	}}

	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeMove,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{"b1": book},
	})

	if len(outcomes) != 1 || !outcomes[0].NoOp || outcomes[0].Applied {
		t.Fatalf("outcomes = %+v, want one NoOp", outcomes)
	}
	if got := workflow.GetStage(book); got != workflow.StageOrganized {
		t.Errorf("GetStage = %v, want StageOrganized (self-healing backfill)", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should be untouched: %v", err)
	}
}

func TestApply_FilelessBookUpdatesPathWithoutFileOp(t *testing.T) {
	book := &library.ComicBook{ID: "b1", FilePath: ""}
	moves := []PlannedMove{{
		BookID: "b1", OldRawPath: "", NewRawPath: `new/placeholder.jpg`,
		OldResolvedPath: "", NewResolvedPath: filepath.Join(t.TempDir(), "placeholder.jpg"),
	}}

	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeMove,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{"b1": book},
	})

	if len(outcomes) != 1 || !outcomes[0].NoOp || outcomes[0].Applied {
		t.Fatalf("outcomes = %+v, want one NoOp", outcomes)
	}
	if book.FilePath != `new/placeholder.jpg` {
		t.Errorf("book.FilePath = %q, want updated NewRawPath", book.FilePath)
	}
}

func TestApply_UnknownBookIDIsFailed(t *testing.T) {
	moves := []PlannedMove{{BookID: "missing", OldResolvedPath: "/x", NewResolvedPath: "/y"}}
	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeMove,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{},
	})
	if len(outcomes) != 1 || !outcomes[0].Failed {
		t.Fatalf("outcomes = %+v, want one Failed", outcomes)
	}
}

func TestApply_SourceMissingIsFailedNotFatal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "does-not-exist.cbz")
	dst := filepath.Join(dir, "new.cbz")

	book := &library.ComicBook{ID: "b1", FilePath: `does-not-exist.cbz`}
	moves := []PlannedMove{{
		BookID: "b1", OldRawPath: `does-not-exist.cbz`, NewRawPath: `new.cbz`,
		OldResolvedPath: src, NewResolvedPath: dst,
	}}

	outcomes := Apply(moves, ApplyOptions{
		Mode:  ModeMove,
		Trash: newTestTrash(t),
		Books: map[string]*library.ComicBook{"b1": book},
	})

	if len(outcomes) != 1 || !outcomes[0].Failed {
		t.Fatalf("outcomes = %+v, want one Failed", outcomes)
	}
	if book.FilePath != `does-not-exist.cbz` {
		t.Errorf("book.FilePath changed despite failure: %q", book.FilePath)
	}
	if got := workflow.GetStage(book); got == workflow.StageOrganized {
		t.Errorf("stage must not advance on a failed apply")
	}
}
