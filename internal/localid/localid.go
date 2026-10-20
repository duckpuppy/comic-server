// Package localid gives a book that ComicVine never lists (typically a
// collected edition) a stable identity of its own (tracker #43).
//
// The identity is two generated keys in one custom value: a volume key that
// sibling books of one line share (Vol 1, Vol 2 ...) and an issue key that is
// unique to the book. It is never written into comicvine_volume or
// comicvine_issue: seeding, completeness and the ComicVine matchers expect
// real numeric ids there. The comic_server_ prefix keeps it out of the
// metadata fingerprint, and ComicRack ignores it.
package localid

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Key is the custom value holding the identity: "<volume key>|<issue key>".
const Key = "comic_server_local_id"

// Identity is a book's local identity.
type Identity struct {
	Volume string `json:"volume_key"`
	Issue  string `json:"issue_key"`
}

func newKey(prefix string) string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		panic("localid: no random source: " + err.Error())
	}
	return prefix + hex.EncodeToString(b)
}

// NewVolumeKey returns a fresh volume key, shared by the books of one line.
func NewVolumeKey() string { return newKey("lv-") }

// NewIssueKey returns a fresh key for one book.
func NewIssueKey() string { return newKey("li-") }

// Get returns the identity stored on b.
func Get(b *library.ComicBook) (Identity, bool) {
	v, _ := library.GetCustomValue(b.CustomValuesStore, Key)
	vol, iss, ok := strings.Cut(v, "|")
	if !ok || !strings.HasPrefix(vol, "lv-") || !strings.HasPrefix(iss, "li-") {
		return Identity{}, false
	}
	return Identity{Volume: vol, Issue: iss}, true
}

// Has reports whether b carries a local identity.
func Has(b *library.ComicBook) bool {
	_, ok := Get(b)
	return ok
}

// Set stores id on b and reports whether anything changed.
func Set(b *library.ComicBook, id Identity) bool {
	val := id.Volume + "|" + id.Issue
	if cur, _ := library.GetCustomValue(b.CustomValuesStore, Key); cur == val {
		return false
	}
	b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, Key, val)
	return true
}

// Clear removes the identity and reports whether there was one.
func Clear(b *library.ComicBook) bool {
	if _, found := library.GetCustomValue(b.CustomValuesStore, Key); !found {
		return false
	}
	b.CustomValuesStore = library.RemoveCustomValue(b.CustomValuesStore, Key)
	return true
}

// SeriesKey normalises a series name for finding sibling books: case,
// surrounding space and repeated spaces do not matter.
func SeriesKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Siblings returns the books that could share b's volume key: same series
// name, a file of their own, and no ComicVine volume. b itself is left out.
// Books that already carry an identity are included (the caller decides).
func Siblings(books []library.ComicBook, b *library.ComicBook) []*library.ComicBook {
	want := SeriesKey(b.Series)
	if want == "" {
		return nil
	}
	var out []*library.ComicBook
	for i := range books {
		o := &books[i]
		if o.ID == b.ID || o.FilePath == "" || SeriesKey(o.Series) != want {
			continue
		}
		if _, tagged := library.GetCustomValue(o.CustomValuesStore, "comicvine_volume"); tagged {
			continue
		}
		out = append(out, o)
	}
	return out
}

// ExistingVolumeKey returns the volume key a sibling already carries, so a
// new volume of a known line joins it. Empty when none has one. When
// siblings disagree the lowest key wins, so the answer is stable.
func ExistingVolumeKey(siblings []*library.ComicBook) string {
	best := ""
	for _, o := range siblings {
		if id, ok := Get(o); ok && (best == "" || id.Volume < best) {
			best = id.Volume
		}
	}
	return best
}
