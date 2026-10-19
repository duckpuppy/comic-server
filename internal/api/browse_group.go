package api

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Browse stacks (tracker #11): the result set grouped by series, publisher,
// year or format, like ComicRack's stacks. Grouping is done on the server over
// the filtered set, so a series never splits across pages; a stack is opened
// by sending its key back as group_key, which narrows the same filter to that
// stack's books.

// browseGroupBys are the accepted group_by values.
var browseGroupBys = map[string]bool{"series": true, "publisher": true, "year": true, "format": true}

// BrowseStack is one group in a grouped Browse result.
type BrowseStack struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Sub     string `json:"sub,omitempty"` // e.g. "Vol 2" or the publisher
	Count   int    `json:"count"`
	Unread  int    `json:"unread"`
	CoverID string `json:"cover_id"`
}

// compressedSeries memoises library.CompressedSeriesName, a pure function whose
// cost (about 8 allocations per call) dominated grouping 67,000 books. Series
// names repeat heavily, so the cache stays small; it is dropped when it passes
// the cap rather than tracked per entry.
var (
	compressedMu    sync.RWMutex
	compressedCache = make(map[string]string, 4096)
)

const compressedCacheCap = 200000

func compressedSeries(series string) string {
	compressedMu.RLock()
	c, ok := compressedCache[series]
	compressedMu.RUnlock()
	if ok {
		return c
	}
	c = library.CompressedSeriesName(series)
	compressedMu.Lock()
	if len(compressedCache) >= compressedCacheCap {
		clear(compressedCache)
	}
	compressedCache[series] = c
	compressedMu.Unlock()
	return c
}

// stackKey returns the stack a book belongs to: a key that is stable across
// requests, a display label and a smaller secondary label.
func stackKey(groupBy string, b *library.ComicBook) (key, label, sub string) {
	switch groupBy {
	case "series":
		key = compressedSeries(b.Series)
		label = b.Series
		if b.Volume > 0 {
			key += "|" + strconv.Itoa(b.Volume)
			sub = "Vol " + strconv.Itoa(b.Volume)
		}
		if key == "" {
			label = "(no series)"
		}
	case "publisher":
		key = strings.ToLower(strings.TrimSpace(b.Publisher))
		label = b.Publisher
		if key == "" {
			label = "(no publisher)"
		}
	case "year":
		if b.Year > 0 {
			key = strconv.Itoa(b.Year)
			label = key
		} else {
			label = "(no year)"
		}
	case "format":
		key = strings.ToLower(strings.TrimSpace(b.Format))
		label = b.Format
		if key == "" {
			label = "(no format)"
		}
	}
	return key, label, sub
}

// issueNumberValue parses a book's issue number the way the number sort does.
func issueNumberValue(b *library.ComicBook) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimLeft(strings.TrimSpace(b.Number), "#"), 64)
	return n, err == nil
}

// filterToStack keeps the books of one stack.
func filterToStack(books []*library.ComicBook, groupBy, groupKey string) []*library.ComicBook {
	out := make([]*library.ComicBook, 0, 32)
	for _, b := range books {
		if k, _, _ := stackKey(groupBy, b); k == groupKey {
			out = append(out, b)
		}
	}
	return out
}

// buildStacks groups books and orders the stacks. sortField "count" orders by
// book count; anything else by label. Stacks with no value ("(no series)")
// always go last. The cover is the stack's lowest numbered issue.
func buildStacks(books []*library.ComicBook, groupBy, sortField string, desc bool) []BrowseStack {
	type acc struct {
		BrowseStack
		coverNum float64
		coverSet bool
	}
	byKey := make(map[string]*acc, 1024)
	order := make([]*acc, 0, 1024)
	for _, b := range books {
		k, label, sub := stackKey(groupBy, b)
		a := byKey[k]
		if a == nil {
			a = &acc{BrowseStack: BrowseStack{Key: k, Label: label, Sub: sub, CoverID: b.ID}}
			byKey[k] = a
			order = append(order, a)
		}
		a.Count++
		if b.IsUnread() {
			a.Unread++
		}
		if n, ok := issueNumberValue(b); ok && (!a.coverSet || n < a.coverNum) {
			a.coverNum, a.coverSet, a.CoverID = n, true, b.ID
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if (a.Key == "") != (b.Key == "") {
			return b.Key == ""
		}
		var less, greater bool
		if sortField == "count" {
			less, greater = a.Count < b.Count, a.Count > b.Count
		} else {
			la, lb := strings.ToLower(a.Label), strings.ToLower(b.Label)
			if groupBy == "year" {
				la, lb = a.Key, b.Key // numeric years: same width in practice
			}
			less, greater = la < lb, la > lb
		}
		if desc {
			return greater
		}
		return less
	})
	stacks := make([]BrowseStack, len(order))
	for i, a := range order {
		stacks[i] = a.BrowseStack
	}
	return stacks
}
