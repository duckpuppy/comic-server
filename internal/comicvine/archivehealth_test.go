package comicvine

import (
	"archive/zip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	rardecode "github.com/nwaples/rardecode/v2"
)

func TestIsCorruptOpenError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"bad zip", zip.ErrFormat, true},
		{"truncated", errors.New("unexpected EOF"), true},
		{"missing file", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}, false},
		{"permission", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrPermission}, false},
		{"io error on a mount", &fs.PathError{Op: "read", Path: "x", Err: errors.New("input/output error")}, false},
		{"password rar", rardecode.ErrBadPassword, false},
	}
	for _, c := range cases {
		if got := IsCorruptOpenError(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOpenFailure_HookFiresOnlyForCorruptArchives(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	SetArchiveFailureHook(func(path string, err error) {
		mu.Lock()
		seen = append(seen, path)
		mu.Unlock()
	})
	t.Cleanup(func() { SetArchiveFailureHook(nil) })

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.cbz")
	if err := os.WriteFile(bad, []byte("this is not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ListPageNames(bad); err == nil {
		t.Fatal("expected an error for a corrupt archive")
	}
	if _, err := ListPageNames(filepath.Join(dir, "missing.cbz")); err == nil {
		t.Fatal("expected an error for a missing file")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != bad {
		t.Errorf("hook calls = %v, want only the corrupt file", seen)
	}
}

func TestArchiveOpenError_KeepsMessageAndUnwraps(t *testing.T) {
	e := &ArchiveOpenError{Format: "cbz", Path: "p", Err: zip.ErrFormat}
	if e.Error() != "open cbz: zip: not a valid zip file" || !errors.Is(e, zip.ErrFormat) {
		t.Errorf("error = %q", e.Error())
	}
}
