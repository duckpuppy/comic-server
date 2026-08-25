package scaninfo

import (
	"os"
	"path/filepath"
	"testing"
)

// loadTestData loads the real scanners.txt/blacklist.txt this package was
// ported against, from testdata (copies of the user's actual ComicRack
// plugin config, added 2026-08-25 for comic-server-pkk.1). Using the real
// lists means tests exercise the exact patterns comic-server needs to be
// compatible with, not a simplified stand-in.
func loadTestData(t *testing.T) (scanners, blacklist []string) {
	t.Helper()
	scanners = loadLines(t, "testdata/scanners.txt")
	blacklist = loadLines(t, "testdata/blacklist.txt")
	return scanners, blacklist
}

func loadLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	var lines []string
	for _, line := range splitLines(string(data)) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func newTestDetector(t *testing.T) *Detector {
	t.Helper()
	scanners, blacklist := loadTestData(t)
	d, err := NewDetector(scanners, blacklist, "Scanner:", "Unknown")
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	return d
}

// Bracket tags matching a scan-group-style name (not necessarily in the
// scanners.txt fallback list) - the primary extraction path, and the more
// important one to get right, since most real scene-tagged filenames have
// this shape. Names here are fictional placeholders, not real scan groups.
func TestDetectTag_BracketHeuristic(t *testing.T) {
	d := newTestDetector(t)

	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "hyphenated group name",
			filename: `Some Series 007 (2016) (Zeta-Fictscans).cbz`,
			want:     "Scanner:Zeta-Fictscans",
		},
		{
			name:     "spaced group name",
			filename: `Another Series 001 (2015) (Son of Placeholder-Group).cbz`,
			want:     "Scanner:Son of Placeholder-Group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := d.DetectTag(Book{FilePath: tt.filename})
			if !ok {
				t.Fatalf("DetectTag() ok = false, want true")
			}
			if got != tt.want {
				t.Errorf("DetectTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectTag_KnownScannerFallback(t *testing.T) {
	d := newTestDetector(t)

	// No bracket/underscore tag at all - falls back to a literal scanners.txt
	// name match anywhere in the filename.
	got, ok := d.DetectTag(Book{FilePath: `Some Series 001 FakeScanCo.cbz`})
	if !ok {
		t.Fatal("DetectTag() ok = false, want true")
	}
	if got != "Scanner:FakeScanCo" {
		t.Errorf("DetectTag() = %q, want %q", got, "Scanner:FakeScanCo")
	}
}

func TestDetectTag_BlacklistedWordFallsBackToUnknown(t *testing.T) {
	d := newTestDetector(t)

	// "Digital" alone is blacklisted (blacklist.txt) and isn't a known
	// scanner name, so this should produce the Unknown fallback tag, not
	// "Scanner:Digital".
	got, ok := d.DetectTag(Book{FilePath: `Some Series 001 (2016) (Digital).cbz`})
	if !ok {
		t.Fatal("DetectTag() ok = false, want true")
	}
	if got != "Scanner:Unknown" {
		t.Errorf("DetectTag() = %q, want %q", got, "Scanner:Unknown")
	}
}

func TestDetectTag_NoUnknownConfigured_SkipsBook(t *testing.T) {
	scanners, blacklist := loadTestData(t)
	d, err := NewDetector(scanners, blacklist, "Scanner:", "") // Unknown disabled
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	_, ok := d.DetectTag(Book{FilePath: `Some Series 001 (2016) (Digital).cbz`})
	if ok {
		t.Error("DetectTag() ok = true, want false when Unknown fallback is disabled and nothing matched")
	}
}

func TestDetectTag_MatchInParentheticalTitle_Rejected(t *testing.T) {
	d := newTestDetector(t)

	// The bracket content "Vol 2" is also the parenthetical part of the
	// book's own Series - must be rejected as a false positive, not
	// mistaken for a scan tag.
	got, ok := d.DetectTag(Book{
		FilePath: `Batman (Vol 2) 001.cbz`,
		Series:   "Batman (Vol 2)",
	})
	if !ok {
		t.Fatal("DetectTag() ok = false, want true (should fall back to Unknown)")
	}
	if got != "Scanner:Unknown" {
		t.Errorf("DetectTag() = %q, want %q (title-parenthetical guard should have rejected the match)", got, "Scanner:Unknown")
	}
}

func TestMergeTag(t *testing.T) {
	tests := []struct {
		name        string
		existing    string
		newTag      string
		wantMerged  string
		wantChanged bool
	}{
		{"empty existing", "", "Scanner:Zone-Empire", "Scanner:Zone-Empire", true},
		{"appends and sorts", "Scanner:Zone-Empire", "Scanner:Alpha", "Scanner:Alpha, Scanner:Zone-Empire", true},
		{"dedup no-op", "Scanner:Zone-Empire", "Scanner:Zone-Empire", "Scanner:Zone-Empire", false},
		// changed=false means "already present" - the raw existing string
		// is intentionally left untouched (not renormalized) to avoid a
		// spurious backend write when nothing actually changed, unlike the
		// original Python plugin which always rewrites/renormalizes on
		// every run regardless of whether the tag was already there.
		{"already-present tag: no write, no reformatting", "Scanner:Zone-Empire ,  Scanner:Alpha", "Scanner:Alpha", "Scanner:Zone-Empire ,  Scanner:Alpha", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, changed := MergeTag(tt.existing, tt.newTag)
			if merged != tt.wantMerged {
				t.Errorf("MergeTag() merged = %q, want %q", merged, tt.wantMerged)
			}
			if changed != tt.wantChanged {
				t.Errorf("MergeTag() changed = %v, want %v", changed, tt.wantChanged)
			}
		})
	}
}

func TestNewDetector_RequiresAtLeastOneScanner(t *testing.T) {
	if _, err := NewDetector(nil, []string{"foo"}, "Scanner:", "Unknown"); err == nil {
		t.Error("expected an error with zero scanners")
	}
}

func TestNewDetector_EmptyBlacklistIsValid(t *testing.T) {
	d, err := NewDetector([]string{"FakeScanCo"}, nil, "Scanner:", "Unknown")
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	got, ok := d.DetectTag(Book{FilePath: filepath.Join("dir", "Some Series 001 FakeScanCo.cbz")})
	if !ok || got != "Scanner:FakeScanCo" {
		t.Errorf("DetectTag() = (%q, %v), want (%q, true)", got, ok, "Scanner:FakeScanCo")
	}
}
