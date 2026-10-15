package comicvine

import "strings"

// PublisherFilter restricts which candidate volumes automatic matching may
// pick, by publisher name (comic-server-2w44). Idea from ComicManager's
// publisher filtering (ideas only): ComicVine lists every regional edition of
// a series, so a search for an American title also returns the Italian or
// Brazilian reprint.
//
// Both lists hold case-insensitive substrings of the publisher name, so
// "Panini" covers "Panini Comics" and "Editorial Panini". Allow narrows the
// field first (empty = allow everything), then Block carves out exceptions. A
// candidate with no publisher recorded always passes: plenty of ComicVine
// volumes have none, and rejecting them would lose real matches.
type PublisherFilter struct {
	Allow []string
	Block []string
}

// Active reports whether the filter would reject anything.
func (f PublisherFilter) Active() bool {
	return len(cleanPublisherTerms(f.Allow)) > 0 || len(cleanPublisherTerms(f.Block)) > 0
}

// Permits reports whether a candidate from publisher may be auto-matched.
func (f PublisherFilter) Permits(publisher string) bool {
	pub := strings.ToLower(strings.TrimSpace(publisher))
	if pub == "" {
		return true
	}
	if allow := cleanPublisherTerms(f.Allow); len(allow) > 0 && !containsAny(pub, allow) {
		return false
	}
	return !containsAny(pub, cleanPublisherTerms(f.Block))
}

// FilterVolumes returns the candidates the filter permits, in order. With no
// active filter it returns candidates unchanged.
func (f PublisherFilter) FilterVolumes(candidates []Volume) []Volume {
	if !f.Active() {
		return candidates
	}
	kept := make([]Volume, 0, len(candidates))
	for _, v := range candidates {
		if f.Permits(v.Publisher.Name) {
			kept = append(kept, v)
		}
	}
	return kept
}

func cleanPublisherTerms(terms []string) []string {
	var out []string
	for _, t := range terms {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func containsAny(s string, terms []string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}
