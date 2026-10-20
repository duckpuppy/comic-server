package comicvine

import "testing"

func TestVolumeIndexResolve(t *testing.T) {
	idx := NewVolumeIndex([]CachedVolume{
		{CVID: 1, Name: "Ghost Rider", StartYear: "2006"},
		{CVID: 2, Name: "Ghost Rider", StartYear: "1990"},
		{CVID: 3, Name: "The Vision", StartYear: "2015"},
		{CVID: 4, Name: "Batman & Robin", StartYear: "2009"},
	})
	cases := []struct {
		title, year string
		wantID      int
		conf        string
	}{
		{"GHOST RIDER", "2006", 1, ResolveHigh},
		{"Ghost Rider", "", 0, ResolveLow},     // two volumes, no year
		{"Ghost Rider", "2001", 0, ResolveLow}, // no volume that year
		{"vision", "", 3, ResolveMedium},       // "The" is ignored
		{"Batman and Robin", "2009", 4, ResolveHigh},
		{"Unknown Title", "", 0, ResolveLow},
		{"", "", 0, ResolveLow},
	}
	for _, c := range cases {
		r := idx.Resolve(c.title, c.year)
		got := 0
		if r.Volume != nil {
			got = r.Volume.CVID
		}
		if got != c.wantID || r.Confidence != c.conf || r.Reason == "" {
			t.Errorf("Resolve(%q,%q) = id %d %s %q, want id %d %s", c.title, c.year, got, r.Confidence, r.Reason, c.wantID, c.conf)
		}
	}
	if r := (*VolumeIndex)(nil).Resolve("x", ""); r.Volume != nil || r.Confidence != ResolveLow {
		t.Errorf("nil index = %+v", r)
	}
}
