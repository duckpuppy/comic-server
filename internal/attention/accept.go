package attention

import (
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// KeyAccepted is the custom value listing the flags the user accepted
// (hid) on one book, "+"-joined (comic-server-x7rd). The comic_server_
// prefix keeps it out of the metadata fingerprint, so accepting a flag
// never makes the file look out of date.
const KeyAccepted = "comic_server_accepted_flags"

// Accepted returns the flags accepted on b.
func Accepted(b *library.ComicBook) map[Flag]bool {
	v, _ := library.GetCustomValue(b.CustomValuesStore, KeyAccepted)
	if v == "" {
		return nil
	}
	out := map[Flag]bool{}
	for _, f := range strings.Split(v, "+") {
		if f = strings.TrimSpace(f); f != "" {
			out[Flag(f)] = true
		}
	}
	return out
}

// SetAccepted accepts (or un-accepts) flag f on b. Returns whether
// anything changed.
func SetAccepted(b *library.ComicBook, f Flag, accepted bool) bool {
	cur := Accepted(b)
	if cur[f] == accepted {
		return false
	}
	if cur == nil {
		cur = map[Flag]bool{}
	}
	if accepted {
		cur[f] = true
	} else {
		delete(cur, f)
	}
	names := make([]string, 0, len(cur))
	for k := range cur {
		names = append(names, string(k))
	}
	sort.Strings(names)
	b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, KeyAccepted, strings.Join(names, "+"))
	return true
}
