// Package countfill fills a book's missing issue Count from the ComicVine
// volume cache (comic-server-08jr). Without a Count, format detection can only
// use keyword and page rules, so most books stay missing_format.
//
// Everything here is pure: Plan reads a book and the cached volume counts and
// returns a proposal. Callers do the writing.
package countfill

import (
	"strconv"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

// KeyAuto holds the Count this package last wrote. A Count that still equals
// it is ours to update when ComicVine's count grows (ongoing series); any
// other non-zero Count was set by hand or by a scraper and is never touched.
// The comic_server_ prefix keeps it out of the CBZ metadata fingerprint.
const KeyAuto = "comic_server_count_auto"

// Proposal is the planned change for one book. Volume is 0 and New is 0 when
// there is nothing to do.
type Proposal struct {
	Volume int
	Old    int
	New    int
}

// Plan returns the Count change for book, given cv_id -> issue_count for the
// synced volumes. It only fills a Count of 0 or replaces one it wrote itself.
// An untagged book or a volume that is not synced is skipped: unknown is not
// zero.
func Plan(book *library.ComicBook, counts map[int]int) (Proposal, bool) {
	vol := comicvine.VolumeIDFromStore(book.CustomValuesStore)
	if vol <= 0 {
		return Proposal{}, false
	}
	n, ok := counts[vol]
	if !ok || n <= 0 || n == book.Count {
		return Proposal{}, false
	}
	if book.Count > 0 {
		auto, _ := library.GetCustomValue(book.CustomValuesStore, KeyAuto)
		if auto != strconv.Itoa(book.Count) {
			return Proposal{}, false // set by hand
		}
	}
	return Proposal{Volume: vol, Old: book.Count, New: n}, true
}

// Apply writes p onto book and records the auto marker.
func Apply(book *library.ComicBook, p Proposal) {
	book.Count = p.New
	book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, KeyAuto, strconv.Itoa(p.New))
}
