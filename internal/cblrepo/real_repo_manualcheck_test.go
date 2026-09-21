//go:build manual

package cblrepo

import (
	"context"
	"testing"
)

// TestRealDieselTechClone is a one-off manual verification against the
// ACTUAL DieselTech/CBL-ReadingLists repo over the real network - not
// part of the normal test suite (build-tagged out, `go test ./...` never
// picks it up) so CI stays hermetic and independent of an external
// repo's uptime. Run explicitly: go test -tags manual ./internal/cblrepo/... -run TestRealDieselTechClone -v
func TestRealDieselTechClone(t *testing.T) {
	clonePath := t.TempDir()
	r := New("https://github.com/DieselTech/CBL-ReadingLists", clonePath)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	files, err := r.ListCBLFiles()
	if err != nil {
		t.Fatalf("ListCBLFiles: %v", err)
	}
	t.Logf("real DieselTech clone: %d .cbl files", len(files))
	if len(files) < 1000 {
		t.Errorf("expected at least 1000 files (repo had 1704 as of 2026-09-20), got %d", len(files))
	}
	commit, err := r.HeadCommit()
	if err != nil || commit == "" {
		t.Fatalf("HeadCommit: commit=%q err=%v", commit, err)
	}
	t.Logf("HEAD: %s", commit)

	batman, err := r.Search("batman")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	t.Logf("search 'batman': %d hits", len(batman))
	if len(batman) == 0 {
		t.Error("expected at least one Batman result")
	}
}
