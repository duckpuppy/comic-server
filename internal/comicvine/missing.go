package comicvine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Per-series missing-issues view (comic-server-buhs): which issues of a
// ComicVine volume the library owns and which it does not, computed from the
// same cache BuildCompletenessMap uses.

// SeriesIssue is one issue of a ComicVine volume.
type SeriesIssue struct {
	IssueCVID int    `json:"issue_id"`
	Number    string `json:"number"`
	Name      string `json:"name,omitempty"`
	CoverDate string `json:"cover_date,omitempty"`
	Owned     bool   `json:"owned"`
	Wanted    bool   `json:"wanted"`            // a placeholder book exists for it
	BookID    string `json:"book_id,omitempty"` // the owning (or wanted) book
}

// SeriesCompleteness is the detail view of one volume.
type SeriesCompleteness struct {
	VolumeID  int           `json:"volume_id"`
	Name      string        `json:"name"`
	Publisher string        `json:"publisher"`
	StartYear string        `json:"start_year"`
	SiteURL   string        `json:"site_url,omitempty"`
	Total     int           `json:"total"`
	Owned     int           `json:"owned"`
	Missing   int           `json:"missing"`
	Wanted    int           `json:"wanted"` // missing issues already on the Wanted list
	Percent   int           `json:"percent"`
	Unmatched int           `json:"unmatched_owned"` // owned books of this volume with no issue id
	Issues    []SeriesIssue `json:"issues"`
}

// VolumeSummary is one row of the volume list.
type VolumeSummary struct {
	VolumeID  int    `json:"volume_id"`
	Name      string `json:"name"`
	Publisher string `json:"publisher"`
	StartYear string `json:"start_year"`
	Total     int    `json:"total"`
	Owned     int    `json:"owned"`
	Missing   int    `json:"missing"`
	Percent   int    `json:"percent"`
}

type volumeHolding struct {
	owned     map[int]string // issue id -> book id
	wanted    map[int]string
	unmatched int
}

func groupHoldings(books []library.ComicBook) map[int]*volumeHolding {
	out := map[int]*volumeHolding{}
	for i := range books {
		b := &books[i]
		vol := extractCVVolumeID(b.CustomValuesStore)
		if vol == 0 {
			continue
		}
		h := out[vol]
		if h == nil {
			h = &volumeHolding{owned: map[int]string{}, wanted: map[int]string{}}
			out[vol] = h
		}
		iss := extractCVIssueID(b.CustomValuesStore)
		switch {
		case library.IsWantedPlaceholder(b):
			if iss > 0 {
				h.wanted[iss] = b.ID
			}
		case iss > 0:
			h.owned[iss] = b.ID
		default:
			h.unmatched++
		}
	}
	return out
}

// BuildSeriesCompleteness lists every issue of volumeID with its ownership.
// It returns nil, nil when the volume's issues are not cached yet.
func BuildSeriesCompleteness(books []library.ComicBook, cache *Cache, volumeID int) (*SeriesCompleteness, error) {
	vol, err := cache.GetVolume(volumeID)
	if err != nil || vol == nil || !vol.IssuesSynced {
		return nil, err
	}
	issues, err := cache.GetIssuesForVolume(volumeID)
	if err != nil {
		return nil, err
	}
	h := groupHoldings(books)[volumeID]
	if h == nil {
		h = &volumeHolding{owned: map[int]string{}, wanted: map[int]string{}}
	}

	sc := &SeriesCompleteness{
		VolumeID: vol.CVID, Name: vol.Name, Publisher: vol.Publisher,
		StartYear: vol.StartYear, SiteURL: vol.SiteURL, Total: len(issues),
		Unmatched: h.unmatched, Issues: make([]SeriesIssue, 0, len(issues)),
	}
	for _, iss := range issues {
		si := SeriesIssue{IssueCVID: iss.CVID, Number: iss.Number, Name: iss.Name, CoverDate: iss.CoverDate}
		if id, ok := h.owned[iss.CVID]; ok {
			si.Owned, si.BookID = true, id
			sc.Owned++
		} else {
			sc.Missing++
			if id, ok := h.wanted[iss.CVID]; ok {
				si.Wanted, si.BookID = true, id
				sc.Wanted++
			}
		}
		sc.Issues = append(sc.Issues, si)
	}
	if sc.Total > 0 {
		sc.Percent = sc.Owned * 100 / sc.Total
	}
	sort.SliceStable(sc.Issues, func(i, j int) bool { return issueNumberLess(sc.Issues[i].Number, sc.Issues[j].Number) })
	return sc, nil
}

// BuildVolumeSummaries summarises every cached volume the library holds at
// least one book of, most-missing first. Volumes whose issue list is not
// synced yet are skipped (their completeness is unknown, not zero).
func BuildVolumeSummaries(books []library.ComicBook, cache *Cache) ([]VolumeSummary, error) {
	var out []VolumeSummary
	for volID, h := range groupHoldings(books) {
		vol, err := cache.GetVolume(volID)
		if err != nil || vol == nil || !vol.IssuesSynced {
			continue
		}
		issues, err := cache.GetIssuesForVolume(volID)
		if err != nil {
			continue
		}
		owned := 0
		for _, iss := range issues {
			if _, ok := h.owned[iss.CVID]; ok {
				owned++
			}
		}
		vs := VolumeSummary{
			VolumeID: vol.CVID, Name: vol.Name, Publisher: vol.Publisher, StartYear: vol.StartYear,
			Total: len(issues), Owned: owned, Missing: len(issues) - owned,
		}
		if vs.Total > 0 {
			vs.Percent = owned * 100 / vs.Total
		}
		out = append(out, vs)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Missing != out[j].Missing {
			return out[i].Missing > out[j].Missing
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// issueNumberLess orders issue numbers numerically where possible
// ("2" < "10" < "10.5"), numeric before non-numeric, then by text.
func issueNumberLess(a, b string) bool {
	fa, ea := strconv.ParseFloat(strings.TrimSpace(a), 64)
	fb, eb := strconv.ParseFloat(strings.TrimSpace(b), 64)
	switch {
	case ea == nil && eb == nil:
		if fa != fb {
			return fa < fb
		}
	case ea == nil:
		return true
	case eb == nil:
		return false
	}
	return strings.ToLower(a) < strings.ToLower(b)
}
