package api

import (
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// BrowseFacets narrows a Browse result by publisher, format, workflow
// stage and attention flag (comic-server-j6lk). Within one facet the
// values are OR'd; facets are AND'd; an empty facet does not filter. Like
// the quick search it only narrows, never widens.
type BrowseFacets struct {
	Publishers []string `json:"publishers,omitempty"` // "" = no publisher
	Formats    []string `json:"formats,omitempty"`    // "" = no format
	Stages     []string `json:"stages,omitempty"`     // workflow.Stage.String(); "" = not staged
	Flags      []string `json:"flags,omitempty"`      // attention.Flag
}

func (f BrowseFacets) empty() bool {
	return len(f.Publishers) == 0 && len(f.Formats) == 0 && len(f.Stages) == 0 && len(f.Flags) == 0
}

// FacetOption is one choice in the filter rail and how many books of the
// current (matchers + quick search) result it would keep.
type FacetOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// FacetCounts is the filter rail's options. Counts are over the result
// before any facet is applied, so picking one option does not make the
// others disappear.
type FacetCounts struct {
	Publishers []FacetOption `json:"publishers"`
	Formats    []FacetOption `json:"formats"`
	Stages     []FacetOption `json:"stages"`
	Flags      []FacetOption `json:"flags"`
}

// bookFlagIndex maps book ID -> its attention flags, from the workflow
// cache, so flag facets match the Needs attention panel exactly (accepted
// flags excluded). Nil when the cache cannot be built.
func (s *Server) bookFlagIndex() map[string][]attention.Flag {
	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		return nil
	}
	idx := map[string][]attention.Flag{}
	for f, books := range entry.booksByFlag {
		for _, b := range books {
			idx[b.ID] = append(idx[b.ID], f)
		}
	}
	return idx
}

func stageKey(b *library.ComicBook) string {
	return workflow.GetStage(b).String()
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// applyFacets returns the books matching f. flags may be nil when no flag
// facet is set.
func applyFacets(books []*library.ComicBook, f BrowseFacets, flags map[string][]attention.Flag) []*library.ComicBook {
	if f.empty() {
		return books
	}
	set := func(vals []string, fold bool) map[string]bool {
		if len(vals) == 0 {
			return nil
		}
		m := make(map[string]bool, len(vals))
		for _, v := range vals {
			v = trimmed(v)
			if fold {
				v = strings.ToLower(v)
			}
			m[v] = true
		}
		return m
	}
	pubs, fmts, stages, wantFlags := set(f.Publishers, true), set(f.Formats, true), set(f.Stages, false), set(f.Flags, false)
	out := books[:0:0]
	for _, b := range books {
		if pubs != nil && !pubs[strings.ToLower(trimmed(b.Publisher))] {
			continue
		}
		if fmts != nil && !fmts[strings.ToLower(trimmed(b.Format))] {
			continue
		}
		if stages != nil && !stages[stageKey(b)] {
			continue
		}
		if wantFlags != nil {
			hit := false
			for _, fl := range flags[b.ID] {
				if wantFlags[string(fl)] {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		out = append(out, b)
	}
	return out
}

// countFacets tallies the rail's options over books. Publisher and format
// values are grouped case-insensitively and shown with the most common
// spelling.
func countFacets(books []*library.ComicBook, flags map[string][]attention.Flag) FacetCounts {
	type tally struct {
		n        int
		spelling map[string]int
	}
	group := func(m map[string]*tally, v string) {
		v = trimmed(v)
		k := strings.ToLower(v)
		t := m[k]
		if t == nil {
			t = &tally{spelling: map[string]int{}}
			m[k] = t
		}
		t.n++
		t.spelling[v]++
	}
	pubs, fmts := map[string]*tally{}, map[string]*tally{}
	stages := map[string]int{}
	flagCounts := map[attention.Flag]int{}
	for _, b := range books {
		group(pubs, b.Publisher)
		group(fmts, b.Format)
		stages[stageKey(b)]++
		for _, f := range flags[b.ID] {
			flagCounts[f]++
		}
	}
	options := func(m map[string]*tally) []FacetOption {
		out := make([]FacetOption, 0, len(m))
		for _, t := range m {
			best, bestN := "", -1
			for sp, n := range t.spelling {
				if n > bestN || (n == bestN && sp < best) {
					best, bestN = sp, n
				}
			}
			label := best
			if label == "" {
				label = "(none)"
			}
			out = append(out, FacetOption{Value: best, Label: label, Count: t.n})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
			return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
		})
		return out
	}
	fc := FacetCounts{Publishers: options(pubs), Formats: options(fmts), Stages: []FacetOption{}, Flags: []FacetOption{}}
	// Stages in pipeline order, then "not staged".
	for _, st := range workflow.Stages {
		if n := stages[st.String()]; n > 0 {
			fc.Stages = append(fc.Stages, FacetOption{Value: st.String(), Label: st.Label(), Count: n})
		}
	}
	if n := stages[""]; n > 0 {
		fc.Stages = append(fc.Stages, FacetOption{Value: "", Label: "Not staged", Count: n})
	}
	// Flags in the Needs attention panel's order (severity, then count).
	for _, sum := range attention.Summarize(flagCounts) {
		fc.Flags = append(fc.Flags, FacetOption{Value: string(sum.Flag), Label: sum.Label, Count: sum.Count})
	}
	return fc
}
