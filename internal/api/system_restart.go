package api

import (
	"net/http"

	"github.com/duckpuppy/comic-server/internal/log"
)

// SetRestartFunc wires the "restart this process" hook into the API server
// (comic-server-9klu). fn must only REQUEST a restart and return promptly -
// cmd/server.go's implementation signals runServer's main loop, which then
// runs the same graceful shutdown/deferred cleanups a SIGTERM would and
// only afterwards re-execs the binary. It must never exec directly from
// the HTTP handler goroutine: that would skip every deferred cleanup
// (backend flush/close, config.db close) and cut off the very response
// that asked for the restart.
//
// Leave this unset on platforms that can't re-exec in place (Windows) -
// POST /api/system/restart then responds 501 and
// GET /api/settings/restart-required reports restart_supported=false, so
// the UI falls back to its manual-restart message.
func (s *Server) SetRestartFunc(fn func() error) {
	s.restartFn = fn
}

// restartSupported reports whether a restart hook is wired in - the single
// source of truth for both the 501 in handleSystemRestart and the
// restart_supported capability flag the UI reads.
func (s *Server) restartSupported() bool {
	return s.restartFn != nil
}

// handleSystemRestart serves POST /api/system/restart. It responds 202
// Accepted first and only then requests the restart, so the client is
// guaranteed to receive the response before the server begins shutting
// down.
func (s *Server) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.restartSupported() {
		http.Error(w, "Restart is not supported on this platform", http.StatusNotImplemented)
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	log.Info().Msg("Restart requested via API")
	if err := s.restartFn(); err != nil {
		// The 202 is already sent; all we can do is log.
		log.Error().Err(err).Msg("Failed to request restart")
	}
}
