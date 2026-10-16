package pathmap

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A ComicRack library is written on Windows, where "Journey into Mystery"
// and "Journey Into Mystery" are the same folder. A Linux disk keeps the
// two apart, so a path recorded with one spelling cannot be opened when the
// folder on disk uses the other (comic-server-bj3j: 500 prod books).
// ExistingCase gives back the real on-disk spelling when the exact path is
// missing and exactly one entry per component matches ignoring case.

const (
	foldCacheTTL  = 5 * time.Minute
	foldCacheDirs = 4096
)

type foldDir struct {
	at    time.Time
	names map[string][]string // lower-case name -> real names
}

var (
	foldMu    sync.Mutex
	foldCache = map[string]foldDir{}
)

// ExistingCase returns p unchanged when it exists, is not an absolute
// slash path, or cannot be matched to exactly one existing path ignoring
// letter case. Otherwise it returns the path as spelled on disk. It never
// invents a path: every component of the result exists.
func ExistingCase(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") {
		return p
	}
	if _, err := os.Lstat(p); err == nil {
		return p
	}
	cur := "/"
	for _, comp := range strings.Split(p, "/") {
		if comp == "" {
			continue
		}
		next := filepath.Join(cur, comp)
		if _, err := os.Lstat(next); err == nil {
			cur = next
			continue
		}
		matches := foldList(cur)[strings.ToLower(comp)]
		if len(matches) != 1 {
			return p // none, or two spellings: never guess
		}
		cur = filepath.Join(cur, matches[0])
	}
	return cur
}

func foldList(dir string) map[string][]string {
	foldMu.Lock()
	if d, ok := foldCache[dir]; ok && time.Since(d.at) < foldCacheTTL {
		foldMu.Unlock()
		return d.names
	}
	foldMu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make(map[string][]string, len(entries))
	for _, e := range entries {
		l := strings.ToLower(e.Name())
		names[l] = append(names[l], e.Name())
	}

	foldMu.Lock()
	if len(foldCache) >= foldCacheDirs {
		foldCache = map[string]foldDir{}
	}
	foldCache[dir] = foldDir{at: time.Now(), names: names}
	foldMu.Unlock()
	return names
}
