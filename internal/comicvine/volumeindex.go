package comicvine

import (
	"fmt"
	"strings"
	"unicode"
)

// VolumeIndex finds a cached ComicVine volume by the name and year a
// description gives (tracker #40). It reads the local cache only: no
// ComicVine call is made, and a series the library has no book of is simply
// not found.
type VolumeIndex struct {
	byName map[string][]CachedVolume
}

// Confidence levels of a Resolution (not the scraper's Confidence values).
const (
	ResolveHigh   = "high"   // one cached volume with this name and year
	ResolveMedium = "medium" // one cached volume with this name, no year to check
	ResolveLow    = "low"    // not resolved
)

// Resolution is the outcome of looking a title up.
type Resolution struct {
	Volume     *CachedVolume // nil when not resolved
	Confidence string
	Reason     string // plain words, shown to the person
}

// NewVolumeIndex indexes volumes by normalised name.
func NewVolumeIndex(vols []CachedVolume) *VolumeIndex {
	idx := &VolumeIndex{byName: make(map[string][]CachedVolume, len(vols))}
	for _, v := range vols {
		k := NormalizeVolumeName(v.Name)
		if k != "" {
			idx.byName[k] = append(idx.byName[k], v)
		}
	}
	return idx
}

// NormalizeVolumeName makes names comparable: case, "&" versus "and", a
// leading "The" and punctuation do not matter.
func NormalizeVolumeName(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "&", " and "))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	s = strings.Join(strings.Fields(b.String()), " ")
	return strings.TrimPrefix(s, "the ")
}

// Resolve looks a title (and optional year) up. Several volumes with the same
// name and no year to tell them apart is "not resolved", never a guess.
func (x *VolumeIndex) Resolve(title, year string) Resolution {
	if strings.TrimSpace(title) == "" {
		return Resolution{Confidence: ResolveLow, Reason: "no series title in the text"}
	}
	if x == nil {
		return Resolution{Confidence: ResolveLow, Reason: "no ComicVine volumes are cached here"}
	}
	found := x.byName[NormalizeVolumeName(title)]
	if len(found) == 0 {
		return Resolution{Confidence: ResolveLow, Reason: fmt.Sprintf("no ComicVine volume named %q is cached here", title)}
	}
	if year != "" {
		var same []CachedVolume
		for _, v := range found {
			if v.StartYear == year {
				same = append(same, v)
			}
		}
		switch len(same) {
		case 1:
			return Resolution{Volume: &same[0], Confidence: ResolveHigh, Reason: fmt.Sprintf("name and start year %s match", year)}
		case 0:
			return Resolution{Confidence: ResolveLow, Reason: fmt.Sprintf("%d cached volume(s) named %q, none started in %s", len(found), title, year)}
		}
		return Resolution{Confidence: ResolveLow, Reason: fmt.Sprintf("%d cached volumes share the name %q and year %s", len(same), title, year)}
	}
	if len(found) == 1 {
		return Resolution{Volume: &found[0], Confidence: ResolveMedium, Reason: "only one cached volume has this name; the text gives no year"}
	}
	return Resolution{Confidence: ResolveLow, Reason: fmt.Sprintf("%d cached volumes are named %q and the text gives no year", len(found), title)}
}
