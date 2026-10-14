package attention

import (
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Flags that compare a book to something other than its own record
// (comic-server-8fee). FlagsFor never returns them: CVFlagsFor needs the
// ComicVine cache, VolumeConflicts needs the whole library.
const (
	FlagCountMismatch    Flag = "count_mismatch"
	FlagMetadataMismatch Flag = "metadata_mismatch"
	FlagCVVolumeConflict Flag = "cv_volume_conflict"
)

// CVIssue is what the ComicVine cache knows about one issue.
type CVIssue struct {
	Volume int
	Number string
}

// CVData is the slice of the ComicVine cache the flags need. A volume or
// issue that is not in it is unknown and never flagged.
type CVData struct {
	Counts map[int]int     // volume ID -> issue count, synced volumes only
	Issues map[int]CVIssue // issue ID -> cached issue
}

func cvID(store, key string) int {
	v, ok := library.GetCustomValue(store, key)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// BookCVIDs returns a book's comicvine_volume and comicvine_issue IDs (0
// when absent).
func BookCVIDs(b *library.ComicBook) (volume, issue int) {
	return cvID(b.CustomValuesStore, "comicvine_volume"), cvID(b.CustomValuesStore, "comicvine_issue")
}

// CVFlagsFor compares one book to the ComicVine cache.
func CVFlagsFor(b *library.ComicBook, cv *CVData) []Flag {
	if cv == nil {
		return nil
	}
	vol, iss := BookCVIDs(b)
	var out []Flag
	// A Count of 0 is missing_count's job, not a mismatch.
	if want, ok := cv.Counts[vol]; ok && vol > 0 && b.Count > 0 && b.Count != want {
		out = append(out, FlagCountMismatch)
	}
	if ci, ok := cv.Issues[iss]; ok && iss > 0 {
		if (vol > 0 && ci.Volume != vol) || normalizeNumber(b.Number) != normalizeNumber(ci.Number) {
			out = append(out, FlagMetadataMismatch)
		}
	}
	return out
}

// VolumeConflicts returns the IDs of books whose ComicVine volume differs
// from the one most of their siblings use. Siblings share series name
// (normalised like the duplicate matcher), Volume and Format; books with
// no Volume are skipped, since one series name with no volume can
// legitimately span several ComicVine volumes. On a tie every tagged
// sibling is returned, because there is no majority to trust.
func VolumeConflicts(books []*library.ComicBook) map[string]bool {
	type sib struct {
		id  string
		vol int
	}
	groups := map[string][]sib{}
	for _, b := range books {
		vol, _ := BookCVIDs(b)
		if vol == 0 || b.Volume <= 0 {
			continue
		}
		series := library.CompressedSeriesName(b.Series)
		if series == "" {
			continue
		}
		key := series + "\x00" + strconv.Itoa(b.Volume) + "\x00" + strings.ToLower(strings.TrimSpace(b.Format))
		groups[key] = append(groups[key], sib{b.ID, vol})
	}
	out := map[string]bool{}
	for _, sibs := range groups {
		counts := map[int]int{}
		for _, s := range sibs {
			counts[s.vol]++
		}
		if len(counts) < 2 {
			continue
		}
		best, bestN, tie := 0, 0, false
		for v, n := range counts {
			switch {
			case n > bestN:
				best, bestN, tie = v, n, false
			case n == bestN:
				tie = true
			}
		}
		for _, s := range sibs {
			if tie || s.vol != best {
				out[s.id] = true
			}
		}
	}
	return out
}

// normalizeNumber lowercases and strips leading zeros so "007" and "7"
// compare equal ("0.5" stays "0.5").
func normalizeNumber(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	if n == "" {
		return ""
	}
	t := strings.TrimLeft(n, "0")
	if t == "" {
		return "0"
	}
	if t[0] < '0' || t[0] > '9' {
		return "0" + t
	}
	return t
}
