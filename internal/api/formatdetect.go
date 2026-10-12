package api

import (
	"fmt"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/formatdetect"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// maxFormatPreview caps the sample rows a preview returns.
const maxFormatPreview = 100

// FormatDetectChange is one proposed Format change.
type FormatDetectChange struct {
	BookID string `json:"book_id"`
	Series string `json:"series"`
	Number string `json:"number"`
	Old    string `json:"old"`
	New    string `json:"new"`
	Reason string `json:"reason"`
}

// FormatDetectPreview is the response for GET .../workflow/format-detect.
type FormatDetectPreview struct {
	Total    int                  `json:"total"`     // books that would change
	ByFormat map[string]int       `json:"by_format"` // proposed Format -> count
	Sample   []FormatDetectChange `json:"sample"`    // first maxFormatPreview
}

// FormatDetectResult is the response for POST .../workflow/format-detect.
type FormatDetectResult struct {
	Applied int `json:"applied"`
}

// planFormats returns every book whose Format detection would change.
func (s *Server) planFormats() ([]*library.ComicBook, []formatdetect.Proposal, error) {
	all, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, nil, err
	}
	var books []*library.ComicBook
	var props []formatdetect.Proposal
	for i := range all {
		if p := formatdetect.Plan(&all[i]); p.Decision == formatdetect.Set {
			books = append(books, &all[i])
			props = append(props, p)
		}
	}
	return books, props, nil
}

// handleFormatDetect previews (GET) or applies (POST) format detection.
// Detection only fills an empty Format or replaces one it wrote itself; a
// Format set by hand or a book locked with comic_server_format_locked=1 is
// never touched.
// GET|POST /api/library/workflow/format-detect
func (s *Server) handleFormatDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	books, props, err := s.planFormats()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load books: %v", err), http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		out := FormatDetectPreview{Total: len(books), ByFormat: map[string]int{}, Sample: []FormatDetectChange{}}
		for i, b := range books {
			out.ByFormat[props[i].Format]++
			if len(out.Sample) < maxFormatPreview {
				out.Sample = append(out.Sample, FormatDetectChange{
					BookID: b.ID, Series: b.Series, Number: b.Number,
					Old: props[i].Old, New: props[i].Format, Reason: props[i].Reason,
				})
			}
		}
		s.writeJSON(w, http.StatusOK, out)
		return
	}

	for i, b := range books {
		formatdetect.Apply(b, props[i])
	}
	if len(books) > 0 {
		if err := s.audited(audit.ActionFormatDetect).UpdateBooks(books); err != nil {
			log.Error().Err(err).Msg("Failed to save format detection")
			http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, FormatDetectResult{Applied: len(books)})
}
