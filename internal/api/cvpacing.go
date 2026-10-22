package api

import (
	"net/http"
	"time"

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// CVPacingResponse is the answer of GET /api/comicvine/pacing.
type CVPacingResponse struct {
	// Configured is false when no ComicVine client is running (no API key).
	Configured bool `json:"configured"`
	comicvine.PacingStatus
	Circuit      string `json:"circuit"`
	CircuitUntil string `json:"circuit_until,omitempty"` // RFC 3339, only while the circuit is open
	// NextRequestSeconds is the longer of the pacer wait and an open circuit's
	// backoff: one number for "when can a request go out" (0 = now).
	NextRequestSeconds int `json:"next_request_seconds"`
}

// handleCVPacing reports where ComicVine pacing stands, for the status chip.
// GET /api/comicvine/pacing
func (s *Server) handleCVPacing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, cvPacingNow())
}

// cvPacingNow builds the pacing answer from the shared pacer and circuit.
func cvPacingNow() CVPacingResponse {
	st, circuit, until, ok := comicvine.SharedPacingStatus()
	resp := CVPacingResponse{Configured: ok, PacingStatus: st, Circuit: circuit.String()}
	resp.NextRequestSeconds = st.WaitSeconds
	if circuit == comicvine.CircuitOpen && until.After(time.Now()) {
		resp.CircuitUntil = until.Format(time.RFC3339)
		resp.NextRequestSeconds = max(resp.NextRequestSeconds, int(time.Until(until).Seconds())+1)
	}
	return resp
}
