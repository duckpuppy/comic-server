package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/log"
)

// CVMatchRequest is the body of POST /api/library/books/:id/cv-match
// (comic-server-5wer). Ref is a pasted ComicVine link or id. Apply false
// previews and changes nothing. Apply true needs the issue: either Ref is an
// issue, or IssueID names the issue picked from a volume's list.
type CVMatchRequest struct {
	Ref     string `json:"ref"`
	IssueID int    `json:"issue_id"`
	Apply   bool   `json:"apply"`
}

// CVMatchBook is the book's own facts, so the preview can be compared with it.
type CVMatchBook struct {
	Series    string `json:"series"`
	Number    string `json:"number"`
	Year      int    `json:"year"`
	PageCount int    `json:"page_count"`
	Format    string `json:"format"`
}

// CVMatchResponse carries a preview (Kind issue or volume) or the applied result.
type CVMatchResponse struct {
	Kind    string                        `json:"kind"`
	Applied bool                          `json:"applied"`
	Book    CVMatchBook                   `json:"book"`
	Issue   *comicvine.IssueMatchPreview  `json:"issue,omitempty"`
	Volume  *comicvine.VolumeMatchPreview `json:"volume,omitempty"`
}

// handleCVMatch matches one book to a ComicVine issue the user named.
// POST /api/library/books/:id/cv-match
func (s *Server) handleCVMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil || s.scraper == nil {
		http.Error(w, "ComicVine scraper is not configured", http.StatusServiceUnavailable)
		return
	}
	id := bookDetailID(r.URL.Path, "/cv-match")
	var req CVMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	book, err := s.backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	resp := CVMatchResponse{Book: CVMatchBook{
		Series: book.Series, Number: book.Number, Year: book.Year,
		PageCount: book.PageCount, Format: book.Format,
	}}

	issueID := req.IssueID
	if req.Ref != "" {
		ref, err := comicvine.ParseCVRef(req.Ref)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if ref.Kind == "volume" {
			if issueID == 0 {
				vp, err := s.scraper.PreviewVolume(r.Context(), ref.ID)
				if err != nil {
					s.writeCVError(w, err)
					return
				}
				resp.Kind, resp.Volume = "volume", vp
				s.writeJSON(w, http.StatusOK, resp)
				return
			}
		} else {
			issueID = ref.ID
		}
	}
	if issueID == 0 {
		http.Error(w, "paste a ComicVine issue link or id", http.StatusBadRequest)
		return
	}

	if !req.Apply {
		ip, err := s.scraper.PreviewIssue(r.Context(), issueID)
		if err != nil {
			s.writeCVError(w, err)
			return
		}
		resp.Kind, resp.Issue = "issue", ip
		s.writeJSON(w, http.StatusOK, resp)
		return
	}

	if book.FilePath == "" {
		http.Error(w, "this book has no file; there is nothing to tag", http.StatusConflict)
		return
	}
	result, err := s.scraper.ApplyIssue(r.Context(), s.audited(audit.ActionCVMatch), id, issueID)
	if err != nil {
		s.writeCVError(w, err)
		return
	}
	s.InvalidateWorkflowCache()
	if ip, err := s.scraper.PreviewIssue(r.Context(), result.IssueID); err == nil {
		resp.Issue = ip
	}
	resp.Kind, resp.Applied = "issue", true
	s.writeJSON(w, http.StatusOK, resp)
}

// writeCVError maps a ComicVine failure to a status the page can show.
func (s *Server) writeCVError(w http.ResponseWriter, err error) {
	var open comicvine.ErrCircuitOpen
	if errors.As(err, &open) {
		http.Error(w, "ComicVine is backing off after an error; try again later ("+err.Error()+")", http.StatusServiceUnavailable)
		return
	}
	log.Warn().Err(err).Msg("cv-match failed")
	http.Error(w, err.Error(), http.StatusBadGateway)
}
