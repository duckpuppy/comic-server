package api

import (
	"encoding/json"
	"net/http"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
)

// ThemeResponse is the GET/PUT /api/settings/theme wire shape.
type ThemeResponse struct {
	Theme string `json:"theme"` // "light", "dark", or "system"
}

// handleThemeConfig serves GET/PUT /api/settings/theme - the default
// theme every brand-new window starts from (comic-server-8qk). A
// per-window toggle override lives in that window's own sessionStorage
// instead (theme.js), never here.
func (s *Server) handleThemeConfig(w http.ResponseWriter, r *http.Request) {
	if s.configDB == nil {
		http.Error(w, "Config database not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetTheme(w, r)
	case http.MethodPut:
		s.handlePutTheme(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetTheme(w http.ResponseWriter, r *http.Request) {
	theme, err := s.configDB.GetTheme()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load theme setting")
		http.Error(w, "Failed to load theme setting", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, ThemeResponse{Theme: theme})
}

func (s *Server) handlePutTheme(w http.ResponseWriter, r *http.Request) {
	var req ThemeResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if !configdb.ValidThemes[req.Theme] {
		http.Error(w, `theme must be one of "light", "dark", "system"`, http.StatusBadRequest)
		return
	}
	if err := s.configDB.SetTheme(req.Theme); err != nil {
		log.Error().Err(err).Msg("Failed to save theme setting")
		http.Error(w, "Failed to save theme setting", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, req)
}
