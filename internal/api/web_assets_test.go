package api

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// TestWebIndexReferencesExistingAssets guards the embedded web UI: a merge
// once left index.html empty (every Go test still passed and the UI was a
// blank page). index.html must be non-empty, and every script and stylesheet
// it links must exist in the embedded filesystem.
func TestWebIndexReferencesExistingAssets(t *testing.T) {
	index, err := fs.ReadFile(webFS, "web/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if len(index) < 1000 || !strings.Contains(string(index), "</html>") {
		t.Fatalf("index.html looks truncated (%d bytes)", len(index))
	}

	refs := regexp.MustCompile(`(?:src|href)="(/(?:js|css)/[^"]+)"`).FindAllStringSubmatch(string(index), -1)
	if len(refs) == 0 {
		t.Fatal("index.html links no scripts or stylesheets")
	}
	for _, m := range refs {
		if _, err := fs.Stat(webFS, "web"+m[1]); err != nil {
			t.Errorf("index.html references %s, which is not embedded: %v", m[1], err)
		}
	}
}
