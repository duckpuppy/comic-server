package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// CSV export of what Browse shows (tracker #13, ComicManager's "CSV list
// export"). The scope is the same BrowseRequest every Browse action uses
// (matchers or show_all, quick search, facets, an open stack), so the file
// holds exactly the books on screen, all of them, not just the loaded page.

// exportColumn is one column a CSV can hold.
type exportColumn struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
	get     func(*library.ComicBook) string
}

// scalarColumn reads a library field by name; a zero number is an empty cell
// (unset), not "0".
func scalarColumn(key, label string, def bool) exportColumn {
	return exportColumn{Key: key, Label: label, Default: def, get: func(b *library.ComicBook) string {
		v, _ := audit.FieldValue(b, key)
		if v == "0" {
			return ""
		}
		return v
	}}
}

var exportColumns = buildExportColumns()

func buildExportColumns() []exportColumn {
	cols := []exportColumn{
		scalarColumn("Series", "Series", true), scalarColumn("Number", "Number", true), scalarColumn("Title", "Title", true),
		scalarColumn("Volume", "Volume", true), scalarColumn("Count", "Count", false), scalarColumn("Year", "Year", true),
		scalarColumn("Month", "Month", false), scalarColumn("Day", "Day", false),
		scalarColumn("Publisher", "Publisher", true), scalarColumn("Imprint", "Imprint", false), scalarColumn("Format", "Format", true),
		scalarColumn("Genre", "Genre", false), scalarColumn("AgeRating", "Age rating", false), scalarColumn("LanguageISO", "Language", false),
		scalarColumn("Writer", "Writer", false), scalarColumn("Penciller", "Penciller", false), scalarColumn("Inker", "Inker", false),
		scalarColumn("Colorist", "Colorist", false), scalarColumn("Letterer", "Letterer", false), scalarColumn("CoverArtist", "Cover artist", false),
		scalarColumn("Editor", "Editor", false), scalarColumn("Characters", "Characters", false), scalarColumn("Teams", "Teams", false),
		scalarColumn("Locations", "Locations", false), scalarColumn("StoryArc", "Story arc", false), scalarColumn("Tags", "Tags", false),
		scalarColumn("Summary", "Summary", false), scalarColumn("Notes", "Notes", false),
		scalarColumn("PageCount", "Pages", true), scalarColumn("Rating", "Rating", false), scalarColumn("CommunityRating", "Community rating", false),
		scalarColumn("BlackAndWhite", "Black and white", false), scalarColumn("Manga", "Manga", false),
		scalarColumn("ISBN", "ISBN", false), scalarColumn("Web", "Web", false), scalarColumn("FileSize", "File size", false),
		{Key: "Read", Label: "Read", Default: true, get: func(b *library.ComicBook) string {
			if b.IsUnread() {
				return "No"
			}
			return "Yes"
		}},
		{Key: "FileName", Label: "File name", Default: true, get: func(b *library.ComicBook) string {
			p := b.FilePath
			if i := strings.LastIndexAny(p, `/\`); i >= 0 {
				p = p[i+1:]
			}
			return p
		}},
		{Key: "FilePath", Label: "File path", get: func(b *library.ComicBook) string { return b.FilePath }},
		{Key: "ComicVineVolume", Label: "ComicVine volume id", get: func(b *library.ComicBook) string {
			v, _ := library.GetCustomValue(b.CustomValuesStore, "comicvine_volume")
			return v
		}},
		{Key: "ComicVineIssue", Label: "ComicVine issue id", get: func(b *library.ComicBook) string {
			v, _ := library.GetCustomValue(b.CustomValuesStore, "comicvine_issue")
			return v
		}},
		{Key: "Stage", Label: "Workflow stage", get: func(b *library.ComicBook) string { return workflow.GetStage(b).String() }},
		{Key: "Kind", Label: "Kind (file / wanted / physical)", get: func(b *library.ComicBook) string { return bookKind(b) }},
	}
	return cols
}

const exportCustomPrefix = "custom:"

var customKeyOK = regexp.MustCompile(`^[A-Za-z0-9_.\- ]{1,64}$`)

// exportColumnFor resolves a requested column key: a catalog column, or
// "custom:<key>" for one of the book's custom values.
func exportColumnFor(key string) (exportColumn, bool) {
	for _, c := range exportColumns {
		if c.Key == key {
			return c, true
		}
	}
	if k, ok := strings.CutPrefix(key, exportCustomPrefix); ok && customKeyOK.MatchString(k) {
		return exportColumn{Key: key, Label: k, get: func(b *library.ComicBook) string {
			v, _ := library.GetCustomValue(b.CustomValuesStore, k)
			return v
		}}, true
	}
	return exportColumn{}, false
}

// csvCell guards against spreadsheet formula injection: a cell that starts
// with = + @ or a tab/CR, or a "-" that is not a plain number, gets a leading
// apostrophe so Excel and Sheets show it as text. "-1" (an issue number) stays.
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	case '-':
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return "'" + s
		}
	}
	return s
}

// BrowseExportRequest is the body of POST /api/library/browse/export-csv.
type BrowseExportRequest struct {
	Browse  BrowseRequest `json:"browse"`
	Columns []string      `json:"columns"` // empty = the default columns
}

// handleBrowseExportColumns lists the columns the export offers.
// GET /api/library/browse/export-columns
func (s *Server) handleBrowseExportColumns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"columns": exportColumns})
}

// handleBrowseExportCSV writes every book in a Browse scope as a CSV download.
// POST /api/library/browse/export-csv
func (s *Server) handleBrowseExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req BrowseExportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	keys := req.Columns
	if len(keys) == 0 {
		for _, c := range exportColumns {
			if c.Default {
				keys = append(keys, c.Key)
			}
		}
	}
	if len(keys) > 100 {
		http.Error(w, "too many columns", http.StatusBadRequest)
		return
	}
	var cols []exportColumn
	seen := map[string]bool{}
	for _, k := range keys {
		c, ok := exportColumnFor(k)
		if !ok {
			http.Error(w, fmt.Sprintf("%q is not a column the export knows", k), http.StatusBadRequest)
			return
		}
		if !seen[c.Key] {
			seen[c.Key] = true
			cols = append(cols, c)
		}
	}

	if req.Browse.MatcherMode == "" {
		req.Browse.MatcherMode = "And"
	}
	books, err := s.resolveBrowseCandidates(req.Browse)
	if err != nil {
		log.Error().Err(err).Msg("Failed to evaluate the export scope")
		http.Error(w, "Failed to evaluate filter", http.StatusInternalServerError)
		return
	}
	if len(books) == 0 {
		http.Error(w, "No books match the current filter, so there is nothing to export", http.StatusConflict)
		return
	}
	sortBrowseResults(books, req.Browse.Sort, req.Browse.SortDesc)

	name := "comic-server-books-" + time.Now().Format("20060102-1504") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A byte order mark makes Excel read the file as UTF-8.
	_, _ = w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Label
	}
	_ = cw.Write(header)
	row := make([]string, len(cols))
	for _, b := range books {
		for i, c := range cols {
			row[i] = csvCell(c.get(b))
		}
		if err := cw.Write(row); err != nil {
			return // the client went away
		}
	}
	cw.Flush()
	log.Info().Int("books", len(books)).Int("columns", len(cols)).Msg("Browse CSV export")
}
