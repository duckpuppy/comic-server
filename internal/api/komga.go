package api

import "net/http"

// handleKomgaStatus returns the most recent Komga sync result for every
// configured target, including any skipped/unmatched books. GET /api/komga/status
func (s *Server) handleKomgaStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.komgaStatus == nil {
		http.Error(w, "Komga sync is not configured", http.StatusServiceUnavailable)
		return
	}

	s.writeJSON(w, http.StatusOK, s.komgaStatus.Snapshot())
}
