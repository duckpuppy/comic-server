package cbl

import (
	"bytes"
	"os"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TestMatchRate_RealLibrary measures the real match rate of the CV-ID +
// string-fallback matcher against the ACTUAL production library (fetched
// read-only over SSH from mediaserver for this measurement per the r1j4
// spec's acceptance criteria - not checked into the repo, not written
// back anywhere). Skipped if the file isn't present locally. This is a
// one-off measurement test, not part of the regular CI matrix's
// intended coverage (it depends on an out-of-repo real-data fixture),
// but living here means `go test ./internal/cbl/...` reproduces it for
// anyone with the file in place.
func TestMatchRate_RealLibrary(t *testing.T) {
	const libPath = "/tmp/claude-1000/-home-duckpuppy-src-comic-server/real-library/ComicDb.xml"
	lib, err := library.LoadLibrary(libPath)
	if err != nil {
		t.Skipf("real library not present locally: %v", err)
	}
	t.Logf("real library: %d books", len(lib.Books))

	books := make([]*library.ComicBook, len(lib.Books))
	for i := range lib.Books {
		books[i] = &lib.Books[i]
	}

	samples := []string{
		"/tmp/claude-1000/-home-duckpuppy-src-comic-server/cbl-samples/batman.cbl",
		"/tmp/claude-1000/-home-duckpuppy-src-comic-server/cbl-samples/elmstreet.cbl",
		"/tmp/claude-1000/-home-duckpuppy-src-comic-server/cbl-samples/wonderland.cbl",
	}

	for _, path := range samples {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("sample not present: %v", err)
		}
		rl, err := Parse(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		var cvHits, stringHits, misses int
		for _, entry := range rl.Books {
			m := MatchEntry(entry, books)
			switch m.Path {
			case MatchCVID:
				cvHits++
			case MatchSeriesNumber:
				stringHits++
			default:
				misses++
			}
		}
		total := len(rl.Books)
		t.Logf("%s (%q, %d entries): cv_id=%d (%.1f%%) string_fallback=%d (%.1f%%) unmatched=%d (%.1f%%)",
			path, rl.Name, total,
			cvHits, pct(cvHits, total),
			stringHits, pct(stringHits, total),
			misses, pct(misses, total))
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}
