package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Bounds on a saved filter: it is a short hand-typed list, not data.
const (
	maxPublisherFilterTerms   = 100
	maxPublisherFilterTermLen = 100
)

// ScrapePublisherFilterResponse is the GET/PUT
// /api/settings/scrape-publisher-filter wire shape. Source says where the
// value in force came from: "saved" (config.db, via this endpoint) or
// "config" (config.yaml / env, never saved in the UI).
type ScrapePublisherFilterResponse struct {
	Allow  []string `json:"allow"`
	Block  []string `json:"block"`
	Source string   `json:"source,omitempty"`
}

// effectiveScrapePublisherFilter returns the filter in force: config.db's
// saved value if the UI ever saved one, else config.yaml / env (comic-server-pl94).
func (s *Server) effectiveScrapePublisherFilter() (comicvine.PublisherFilter, string) {
	if s.configDB != nil {
		stored, err := s.configDB.GetScrapePublisherFilter()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to read scrape publisher filter; using config")
		} else if stored != nil {
			return comicvine.PublisherFilter{Allow: stored.Allow, Block: stored.Block}, "saved"
		}
	}
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	if s.config == nil {
		return comicvine.PublisherFilter{}, "config"
	}
	return comicvine.PublisherFilter{
		Allow: s.config.Server.ScrapePublisherAllow,
		Block: s.config.Server.ScrapePublisherBlock,
	}, "config"
}

// cleanFilterTerms trims, drops blanks and case-insensitive duplicates.
func cleanFilterTerms(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

// handleScrapePublisherFilter serves GET/PUT /api/settings/scrape-publisher-filter.
func (s *Server) handleScrapePublisherFilter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f, source := s.effectiveScrapePublisherFilter()
		s.writeJSON(w, http.StatusOK, ScrapePublisherFilterResponse{
			Allow: cleanFilterTerms(f.Allow), Block: cleanFilterTerms(f.Block), Source: source,
		})
	case http.MethodPut:
		if s.configDB == nil {
			http.Error(w, "Config database not available", http.StatusServiceUnavailable)
			return
		}
		var req ScrapePublisherFilterResponse
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		allow, block := cleanFilterTerms(req.Allow), cleanFilterTerms(req.Block)
		if len(allow) > maxPublisherFilterTerms || len(block) > maxPublisherFilterTerms {
			http.Error(w, "Too many entries", http.StatusBadRequest)
			return
		}
		for _, t := range append(append([]string{}, allow...), block...) {
			if len(t) > maxPublisherFilterTermLen {
				http.Error(w, "An entry is too long", http.StatusBadRequest)
				return
			}
		}
		if err := s.configDB.UpsertScrapePublisherFilter(configdb.ScrapePublisherFilter{Allow: allow, Block: block}); err != nil {
			log.Error().Err(err).Msg("Failed to save scrape publisher filter")
			http.Error(w, "Failed to save scrape publisher filter", http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, http.StatusOK, ScrapePublisherFilterResponse{Allow: allow, Block: block, Source: "saved"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
