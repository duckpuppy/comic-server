package comicvine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/collects"
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
	// Covered: no book of its own, but a collected edition contains it
	// (tracker #41). CoveredBy is that collected book. Owned wins over covered.
	Covered   bool   `json:"covered,omitempty"`
	CoveredBy string `json:"covered_by,omitempty"`
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
	Covered   int           `json:"covered"` // not owned, but inside a collected edition the library holds
	Missing   int           `json:"missing"` // neither owned nor covered
	Wanted    int           `json:"wanted"`  // missing issues already on the Wanted list
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
	Covered   int    `json:"covered"`
	Missing   int    `json:"missing"`
	Percent   int    `json:"percent"`
}

type volumeHolding struct {
	owned     map[int]string // issue id -> book id
	wanted    map[int]string
	unmatched int
	runs      []collectionRun // collected editions that contain issues of this volume
}

// collectionRun is one whole-issue run of a collected edition (tracker #41).
type collectionRun struct {
	bookID string
	entry  collects.Entry
}

func newHolding() *volumeHolding {
	return &volumeHolding{owned: map[int]string{}, wanted: map[int]string{}}
}

func groupHoldings(books []library.ComicBook) map[int]*volumeHolding {
	out := map[int]*volumeHolding{}
	for i := range books {
		b := &books[i]
		addCollectionRuns(out, b)
		vol := extractCVVolumeID(b.CustomValuesStore)
		if vol == 0 {
			continue
		}
		h := out[vol]
		if h == nil {
			h = newHolding()
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

// addCollectionRuns records the whole-issue runs of a collected book. A
// backup story or a partial issue does not cover an issue, a Wanted
// placeholder holds nothing, and an entry with no ComicVine volume cannot be
// resolved (unknown is not covered).
func addCollectionRuns(out map[int]*volumeHolding, b *library.ComicBook) {
	if library.IsWantedPlaceholder(b) {
		return
	}
	for _, e := range collects.Get(b) {
		if e.Volume <= 0 || e.Kind == collects.KindBackup || e.Partial {
			continue
		}
		h := out[e.Volume]
		if h == nil {
			h = newHolding()
			out[e.Volume] = h
		}
		h.runs = append(h.runs, collectionRun{bookID: b.ID, entry: e})
	}
}

// coveredIssues maps issue id -> collected book id for the issues of a
// volume that a collection contains. A range with a missing end, or one that
// runs backwards, covers nothing.
func (h *volumeHolding) coveredIssues(issues []CachedIssue) map[int]string {
	out := map[int]string{}
	if len(h.runs) == 0 {
		return out
	}
	sorted := append([]CachedIssue(nil), issues...)
	sort.SliceStable(sorted, func(i, j int) bool { return issueNumberLess(sorted[i].Number, sorted[j].Number) })
	for _, r := range h.runs {
		for _, id := range coveredByEntry(r.entry.Normalize(), sorted) {
			if _, ok := out[id]; !ok {
				out[id] = r.bookID
			}
		}
	}
	return out
}

// coveredByEntry returns the ids of the issues (sorted by number) inside one run.
func coveredByEntry(e collects.Entry, sorted []CachedIssue) []int {
	var ids []int
	if e.To == "" {
		want := collects.NormNumber(e.From)
		for _, iss := range sorted {
			if collects.NormNumber(iss.Number) == want {
				ids = append(ids, iss.CVID)
			}
		}
		return ids
	}
	from, errA := strconv.ParseFloat(e.From, 64)
	to, errB := strconv.ParseFloat(e.To, 64)
	if errA == nil && errB == nil {
		for _, iss := range sorted {
			if n, err := strconv.ParseFloat(strings.TrimSpace(iss.Number), 64); err == nil && n >= from && n <= to {
				ids = append(ids, iss.CVID)
			}
		}
		return ids
	}
	// Text endpoints ("Annual 1" to "Annual 3"): everything between the two
	// in number order, when both exist in the volume.
	i, j := -1, -1
	for k, iss := range sorted {
		switch collects.NormNumber(iss.Number) {
		case collects.NormNumber(e.From):
			if i < 0 {
				i = k
			}
		case collects.NormNumber(e.To):
			j = k
		}
	}
	if i < 0 || j < i {
		return nil
	}
	for _, iss := range sorted[i : j+1] {
		ids = append(ids, iss.CVID)
	}
	return ids
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
		h = newHolding()
	}
	covered := h.coveredIssues(issues)

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
		} else if by, ok := covered[iss.CVID]; ok {
			si.Covered, si.CoveredBy = true, by
			sc.Covered++
			if id, ok := h.wanted[iss.CVID]; ok {
				si.Wanted, si.BookID = true, id
			}
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
		owned, coveredN := 0, 0
		covered := h.coveredIssues(issues)
		for _, iss := range issues {
			if _, ok := h.owned[iss.CVID]; ok {
				owned++
			} else if _, ok := covered[iss.CVID]; ok {
				coveredN++
			}
		}
		vs := VolumeSummary{
			VolumeID: vol.CVID, Name: vol.Name, Publisher: vol.Publisher, StartYear: vol.StartYear,
			Total: len(issues), Owned: owned, Covered: coveredN, Missing: len(issues) - owned - coveredN,
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
