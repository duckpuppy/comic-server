package pathmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingCase(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "DC", "Wildstorm", "Wild Storm (2017)"), 0o755))
	file := filepath.Join(root, "DC", "Wildstorm", "Wild Storm (2017)", "Issue 05.cbz")
	must(os.WriteFile(file, []byte("x"), 0o644))
	// two spellings of one folder: must never be guessed
	must(os.MkdirAll(filepath.Join(root, "Both", "alpha"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "Both", "ALPHA"), 0o755))

	cases := []struct{ name, in, want string }{
		{"exact path is untouched", file, file},
		{"folder case differs", filepath.Join(root, "DC", "WildStorm", "Wild Storm (2017)", "Issue 05.cbz"), file},
		{"file case differs", filepath.Join(root, "DC", "Wildstorm", "Wild Storm (2017)", "ISSUE 05.CBZ"), file},
		{"several components differ", filepath.Join(root, "dc", "WILDSTORM", "wild storm (2017)", "issue 05.cbz"), file},
		{"missing file stays as given", filepath.Join(root, "DC", "WildStorm", "Wild Storm (2017)", "Nope.cbz"), filepath.Join(root, "DC", "WildStorm", "Wild Storm (2017)", "Nope.cbz")},
		{"two spellings are not guessed", filepath.Join(root, "Both", "Alpha", "x.cbz"), filepath.Join(root, "Both", "Alpha", "x.cbz")},
		{"windows path is untouched", `G:\Comics\A.cbz`, `G:\Comics\A.cbz`},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExistingCase(c.in); got != c.want {
				t.Errorf("ExistingCase(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
