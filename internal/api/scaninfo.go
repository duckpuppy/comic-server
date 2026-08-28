package api

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/scaninfo"
)

// ScanInfoResult is the response for POST .../scan-info.
type ScanInfoResult struct {
	Processed int      `json:"processed"`
	Updated   int      `json:"updated"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
}

// handleRunScanInfo detects and writes ScanInformation for every book
// currently matched by one smart list (comic-server-pkk.1) - mirroring how
// the user manually selects a smart list's results in ComicRack and runs
// the ScanInformationFromFilename plugin over them. A fresh Detector is
// built per request from current config; scanners/blacklist rarely change,
// and this endpoint isn't hot-path, so there's no need to cache it.
// POST /api/library/lists/:listId/scan-info
func (s *Server) handleRunScanInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := s.effectiveScanInfo()
	if err != nil {
		log.Error().Err(err).Msg("Failed to load scan info config")
		http.Error(w, "Failed to load scan info config", http.StatusInternalServerError)
		return
	}

	if !cfg.Enabled {
		http.Error(w, "scan_info is not enabled in config", http.StatusServiceUnavailable)
		return
	}

	detector, err := scaninfo.NewDetector(cfg.Scanners, cfg.Blacklist, cfg.Prefix, cfg.Unknown)
	if err != nil {
		log.Error().Err(err).Msg("Failed to build scan-info detector from config")
		http.Error(w, "Invalid scan_info configuration", http.StatusInternalServerError)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}

	listID := listIDFromScanInfoSubPath(r.URL.Path)
	list, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up smart list for scan-info run")
		http.Error(w, "Error looking up smart list", http.StatusInternalServerError)
		return
	}
	if list == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	books, err := s.backend.MatchBooks(list)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to match books: %v", err), http.StatusInternalServerError)
		return
	}

	result := ScanInfoResult{Processed: len(books)}
	var toUpdate []*library.ComicBook

	for _, book := range books {
		tag, ok := detector.DetectTag(toScanInfoBook(book))
		if !ok {
			result.Skipped++
			continue
		}
		merged, changed := scaninfo.MergeTag(book.ScanInformation, tag)
		if !changed {
			result.Skipped++
			continue
		}
		book.ScanInformation = merged
		toUpdate = append(toUpdate, book)
		result.Updated++
	}

	if len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save scan-info updates")
			result.Errors = append(result.Errors, err.Error())
		}
	}

	s.writeJSON(w, http.StatusOK, result)
}

func listIDFromScanInfoSubPath(path string) string {
	suffix := strings.TrimPrefix(path, "/api/library/lists/")
	return strings.TrimSuffix(suffix, "/scan-info")
}

// toScanInfoBook builds a scaninfo.Book from a library.ComicBook, including
// every other populated string field as OtherFields for the
// false-positive guard - comic-server's stand-in for ComicRack's
// GetComicFields() reflection (see scaninfo.DetectTag's guard 1).
func toScanInfoBook(book *library.ComicBook) scaninfo.Book {
	var other []string
	v := reflect.ValueOf(*book)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() != reflect.String {
			continue
		}
		switch field.Name {
		case "ID", "FilePath", "ScanInformation":
			continue
		}
		if val := v.Field(i).String(); val != "" {
			other = append(other, val)
		}
	}

	return scaninfo.Book{
		FilePath:        book.FilePath,
		Series:          book.Series,
		Title:           book.Title,
		AlternateSeries: book.AlternateSeries,
		ScanInformation: book.ScanInformation,
		OtherFields:     other,
	}
}
