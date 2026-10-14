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
	FlagFormatMismatch   Flag = "format_mismatch_in_volume"
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
	groups := map[string][]member{}
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
		groups[key] = append(groups[key], member{b.ID, strconv.Itoa(vol)})
	}
	return minorities(groups)
}

// FormatMismatches returns the IDs of books whose Format differs from the
// one most books of the same ComicVine volume use (comic-server-0vu9),
// e.g. one issue marked "Limited Series" in a run marked "Series". An
// empty Format is missing_format's job and is not compared. Ties return
// every book with a Format.
func FormatMismatches(books []*library.ComicBook) map[string]bool {
	groups := map[string][]member{}
	for _, b := range books {
		vol, _ := BookCVIDs(b)
		format := strings.ToLower(strings.TrimSpace(b.Format))
		if vol == 0 || format == "" {
			continue
		}
		key := strconv.Itoa(vol)
		groups[key] = append(groups[key], member{b.ID, format})
	}
	return minorities(groups)
}

// member is one book in a sibling group and the value being compared.
type member struct {
	id    string
	value string
}

// minorities returns the IDs of members whose value is not their group's
// majority value. A group with one value contributes nothing; a tie for
// the majority contributes every member.
func minorities(groups map[string][]member) map[string]bool {
	out := map[string]bool{}
	for _, ms := range groups {
		counts := map[string]int{}
		for _, m := range ms {
			counts[m.value]++
		}
		if len(counts) < 2 {
			continue
		}
		best, bestN, tie := "", 0, false
		for v, n := range counts {
			switch {
			case n > bestN:
				best, bestN, tie = v, n, false
			case n == bestN:
				tie = true
			}
		}
		for _, m := range ms {
			if tie || m.value != best {
				out[m.id] = true
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
