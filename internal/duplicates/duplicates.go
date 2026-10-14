// Package duplicates detects groups of library books that look like
// duplicates of each other, for the duplicate review screen
// (comic-server-as8l). Detection is pure (no I/O): cover hashing is
// injected so callers decide whether and how to pay for it.
//
// Signals, in priority order (a group may carry several reasons):
//  1. same ComicVine issue ID
//  2. same series + number + volume + format (leading zeros ignored, series
//     normalised exactly like ComicBookDuplicateMatcher)
//  3. visually near-identical cover (dHash Hamming distance, anchor-based
//     so matches do not chain), only within one series
//  4. OS-style copy filenames: "Comic (1).cbz", "Comic - Copy.cbz"
//
// Guards: a group whose largest page count is more than 3x its smallest is
// dropped (protects a TPB from being flagged against a single issue). A
// fileless book (ComicRack's physical-copy catalog entry, or a Wanted
// placeholder) never groups with a book that has a file: owning the
// physical and the digital copy is not a duplicate.
package duplicates

import (
	"crypto/sha1"
	"encoding/hex"
	"math/bits"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Reason says why books were grouped.
type Reason string

const (
	ReasonComicVineIssue Reason = "comicvine_issue"
	ReasonSeriesNumber   Reason = "series_number"
	ReasonCover          Reason = "cover"
	ReasonFilenameCopy   Reason = "filename_copy"
)

// DefaultHammingThreshold is the max differing bits (of 64) for two covers
// to count as the same image.
const DefaultHammingThreshold = 5

// MaxPageRatio is the page-count guard: groups spanning more than this
// ratio are dropped.
const MaxPageRatio = 3

// Group is one set of suspected duplicates.
type Group struct {
	Key     string // stable hash of the sorted member IDs
	Reasons []Reason
	Books   []*library.ComicBook
}

// Options configures Detect.
type Options struct {
	// CoverHash returns a 64-bit cover hash for a book, or ok=false when
	// unavailable. Nil disables the cover signal.
	CoverHash func(b *library.ComicBook) (hash uint64, ok bool)
	// HammingThreshold defaults to DefaultHammingThreshold when 0.
	HammingThreshold int
}

// GroupKey returns the stable key for a set of book IDs.
func GroupKey(ids []string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	sum := sha1.Sum([]byte(strings.Join(sorted, "\x00")))
	return hex.EncodeToString(sum[:8])
}

type unionFind struct{ parent []int }

func newUF(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{p}
}

func (u *unionFind) find(i int) int {
	for u.parent[i] != i {
		u.parent[i] = u.parent[u.parent[i]]
		i = u.parent[i]
	}
	return i
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

// Detect returns suspected-duplicate groups (size >= 2) among books.
// Callers exclude already-resolved books before calling.
func Detect(books []*library.ComicBook, opt Options) []Group {
	n := len(books)
	uf := newUF(n)
	// reasons recorded per member index, merged per final group.
	reasons := make([]map[Reason]bool, n)
	note := func(idxs []int, r Reason) {
		for i, ix := range idxs {
			if reasons[ix] == nil {
				reasons[ix] = map[Reason]bool{}
			}
			reasons[ix][r] = true
			if i > 0 {
				uf.union(idxs[0], ix)
			}
		}
	}
	flush := func(buckets map[string][]int, r Reason) {
		for _, idxs := range buckets {
			if len(idxs) > 1 {
				note(idxs, r)
			}
		}
	}

	// 1. ComicVine issue ID.
	cv := map[string][]int{}
	for i, b := range books {
		if id, ok := library.GetCustomValue(b.CustomValuesStore, "comicvine_issue"); ok && id != "" && id != "0" {
			key := kind(b) + id
			cv[key] = append(cv[key], i)
		}
	}
	flush(cv, ReasonComicVineIssue)

	// 2. Series + number (+ volume, format).
	sn := map[string][]int{}
	seriesBuckets := map[string][]int{}
	for i, b := range books {
		series := library.CompressedSeriesName(b.Series)
		if series == "" {
			continue
		}
		seriesBuckets[series] = append(seriesBuckets[series], i)
		num := normalizeNumber(b.Number)
		if num == "" {
			continue
		}
		key := kind(b) + series + "\x00" + num + "\x00" + itoa(b.Volume) + "\x00" + strings.ToLower(b.Format)
		sn[key] = append(sn[key], i)
	}
	flush(sn, ReasonSeriesNumber)

	// 3. Cover hash, within one series only (bounds the cost and the
	// false-positive rate), anchor-based so matches never chain.
	if opt.CoverHash != nil {
		thr := opt.HammingThreshold
		if thr <= 0 {
			thr = DefaultHammingThreshold
		}
		for _, idxs := range seriesBuckets {
			if len(idxs) < 2 {
				continue
			}
			type hb struct {
				idx  int
				hash uint64
			}
			var hashed []hb
			for _, ix := range idxs {
				if h, ok := opt.CoverHash(books[ix]); ok {
					hashed = append(hashed, hb{ix, h})
				}
			}
			used := make([]bool, len(hashed))
			for a := range hashed {
				if used[a] {
					continue
				}
				members := []int{hashed[a].idx}
				for c := a + 1; c < len(hashed); c++ {
					if !used[c] && bits.OnesCount64(hashed[a].hash^hashed[c].hash) <= thr {
						used[c] = true
						members = append(members, hashed[c].idx)
					}
				}
				used[a] = true
				if len(members) > 1 {
					note(members, ReasonCover)
				}
			}
		}
	}

	// 4. Copy-style filenames: group by dir + stem with the copy marker
	// stripped, only when at least one member actually has a marker.
	fn := map[string][]int{}
	hasCopy := map[string]bool{}
	for i, b := range books {
		if b.FilePath == "" {
			continue
		}
		p := strings.ReplaceAll(b.FilePath, "\\", "/")
		base := path.Base(p)
		stem := strings.TrimSuffix(base, path.Ext(base))
		clean, isCopy := stripCopyMarker(stem)
		key := strings.ToLower(path.Dir(p) + "/" + clean)
		fn[key] = append(fn[key], i)
		if isCopy {
			hasCopy[key] = true
		}
	}
	for k, idxs := range fn {
		if len(idxs) > 1 && hasCopy[k] {
			note(idxs, ReasonFilenameCopy)
		}
	}

	// Collect final groups.
	byRoot := map[int][]int{}
	for i := range books {
		if reasons[i] != nil {
			byRoot[uf.find(i)] = append(byRoot[uf.find(i)], i)
		}
	}
	var out []Group
	for _, idxs := range byRoot {
		if len(idxs) < 2 {
			continue
		}
		g := Group{}
		rs := map[Reason]bool{}
		ids := make([]string, 0, len(idxs))
		minP, maxP := 0, 0
		for _, ix := range idxs {
			b := books[ix]
			g.Books = append(g.Books, b)
			ids = append(ids, b.ID)
			for r := range reasons[ix] {
				rs[r] = true
			}
			if b.PageCount > 0 {
				if minP == 0 || b.PageCount < minP {
					minP = b.PageCount
				}
				if b.PageCount > maxP {
					maxP = b.PageCount
				}
			}
		}
		if minP > 0 && maxP > MaxPageRatio*minP {
			continue
		}
		for _, r := range []Reason{ReasonComicVineIssue, ReasonSeriesNumber, ReasonCover, ReasonFilenameCopy} {
			if rs[r] {
				g.Reasons = append(g.Reasons, r)
			}
		}
		sort.Slice(g.Books, func(i, j int) bool {
			if g.Books[i].FilePath != g.Books[j].FilePath {
				return g.Books[i].FilePath < g.Books[j].FilePath
			}
			return g.Books[i].ID < g.Books[j].ID
		})
		g.Key = GroupKey(ids)
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Books[0], out[j].Books[0]
		sa, sb := strings.ToLower(a.Series), strings.ToLower(b.Series)
		if sa != sb {
			return sa < sb
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// kind prefixes a bucket key so fileless and file-backed books never share
// a bucket. The cover and filename signals need a file, so they cannot
// mix the two kinds anyway.
func kind(b *library.ComicBook) string {
	if b.FilePath == "" {
		return "p\x00"
	}
	return "f\x00"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// normalizeNumber lowercases and strips leading zeros ("007" -> "7",
// "000" -> "0", "01a" -> "1a").
func normalizeNumber(num string) string {
	num = strings.ToLower(strings.TrimSpace(num))
	if num == "" {
		return ""
	}
	t := strings.TrimLeft(num, "0")
	if t == "" {
		return "0"
	}
	if t[0] < '0' || t[0] > '9' {
		return "0" + t // "0.5" must stay "0.5", not ".5"
	}
	return t
}

var (
	copyParenRe = regexp.MustCompile(`^(.*?)\s*\(\d{1,2}\)$`)
	copyWordRe  = regexp.MustCompile(`(?i)^(.*?)\s*[-_ ]\s*copy(?:\s*\(\d+\)|\s*\d+)?$`)
	copyOfRe    = regexp.MustCompile(`(?i)^copy (?:\(\d+\) )?of (.*)$`)
)

// stripCopyMarker removes an OS-style copy marker from a file stem.
func stripCopyMarker(stem string) (string, bool) {
	for _, re := range []*regexp.Regexp{copyParenRe, copyWordRe, copyOfRe} {
		if m := re.FindStringSubmatch(stem); m != nil && strings.TrimSpace(m[1]) != "" {
			return strings.TrimSpace(m[1]), true
		}
	}
	return stem, false
}
