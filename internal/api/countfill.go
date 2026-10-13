package api

import (
	"fmt"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/countfill"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// CountFillChange is one proposed Count change.
type CountFillChange struct {
	BookID string `json:"book_id"`
	Series string `json:"series"`
	Number string `json:"number"`
	Volume int    `json:"volume"`
	Old    int    `json:"old"`
	New    int    `json:"new"`
}

// CountFillPreview is the response for GET .../workflow/count-fill.
type CountFillPreview struct {
	Total   int               `json:"total"`   // books that would change
	Volumes int               `json:"volumes"` // distinct volumes among them
	Waiting int               `json:"waiting"` // Count 0, tagged, volume not synced yet
	Sample  []CountFillChange `json:"sample"`  // first maxFormatPreview
}

// CountFillResult is the response for POST .../workflow/count-fill.
type CountFillResult struct {
	Applied int `json:"applied"`
}

// planCounts returns every book whose Count would change, and how many
// zero-Count tagged books still wait for their volume to sync.
func (s *Server) planCounts() ([]*library.ComicBook, []countfill.Proposal, int, error) {
	counts, err := s.cvCache.SyncedIssueCounts()
	if err != nil {
		return nil, nil, 0, err
	}
	all, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, nil, 0, err
	}
	var books []*library.ComicBook
	var props []countfill.Proposal
	waiting := 0
	for i := range all {
		b := &all[i]
		if p, ok := countfill.Plan(b, counts); ok {
			books = append(books, b)
			props = append(props, p)
			continue
		}
		if b.Count == 0 {
			if vol := comicvine.VolumeIDFromStore(b.CustomValuesStore); vol > 0 {
				if _, synced := counts[vol]; !synced {
					waiting++
				}
			}
		}
	}
	return books, props, waiting, nil
}

// handleCountFill previews (GET) or applies (POST) filling Count from the
// ComicVine volume cache. It only fills a Count of 0 or updates one it wrote
// itself; a Count set by hand is never touched.
// GET|POST /api/library/workflow/count-fill
func (s *Server) handleCountFill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.cvCache == nil {
		http.Error(w, "ComicVine cache not available", http.StatusServiceUnavailable)
		return
	}
	books, props, waiting, err := s.planCounts()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to plan counts: %v", err), http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		out := CountFillPreview{Total: len(books), Waiting: waiting, Sample: []CountFillChange{}}
		vols := map[int]bool{}
		for i, b := range books {
			vols[props[i].Volume] = true
			if len(out.Sample) < maxFormatPreview {
				out.Sample = append(out.Sample, CountFillChange{
					BookID: b.ID, Series: b.Series, Number: b.Number,
					Volume: props[i].Volume, Old: props[i].Old, New: props[i].New,
				})
			}
		}
		out.Volumes = len(vols)
		s.writeJSON(w, http.StatusOK, out)
		return
	}

	for i, b := range books {
		countfill.Apply(b, props[i])
	}
	if len(books) > 0 {
		if err := s.audited(audit.ActionCountFill).UpdateBooks(books); err != nil {
			log.Error().Err(err).Msg("Failed to save count fill")
			http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, CountFillResult{Applied: len(books)})
}
