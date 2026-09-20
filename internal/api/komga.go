package api

import (
	"net/http"

	"github.com/duckpuppy/comic-server/internal/komga"
)

// komgaStatusResponse wraps komga.Snapshot with a "configured" flag.
// "Not configured" is a normal state (Komga sync is optional), not an
// error - it used to be signaled with a 503, which the browser logs as a
// console error on every page load of an unconfigured server regardless
// of how the JS handles it (comic-server-hono). Reporting it as 200 body
// data instead keeps the console clean.
type komgaStatusResponse struct {
	Configured bool `json:"configured"`
	komga.Snapshot
}

// handleKomgaStatus returns the most recent Komga sync result for every
// configured target, including any skipped/unmatched books. GET /api/komga/status
func (s *Server) handleKomgaStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.komgaStatus == nil {
		s.writeJSON(w, http.StatusOK, komgaStatusResponse{Configured: false})
		return
	}

	s.writeJSON(w, http.StatusOK, komgaStatusResponse{Configured: true, Snapshot: s.komgaStatus.Snapshot()})
}
