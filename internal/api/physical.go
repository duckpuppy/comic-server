package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
)

// Paper copies (comic-server-vffq): a book with no file whose Collection
// Status is "Physical" (library.IsPhysical). They count as owned in
// completeness, never enter the workflow, and are not Wanted.
//
// POST /api/library/physical          add one (PhysicalBook)
// POST /api/library/physical/import   CSV import (PhysicalImportRequest)

// PhysicalBook is one paper copy to add, from the form or a CSV row.
type PhysicalBook struct {
	Series          string `json:"series"`
	Number          string `json:"number"`
	Volume          int    `json:"volume"`
	Year            int    `json:"year"`
	Publisher       string `json:"publisher"`
	Title           string `json:"title"`
	Format          string `json:"format"`
	Location        string `json:"location"`
	Condition       string `json:"condition"`
	Notes           string `json:"notes"`
	ComicVineVolume int    `json:"comicvine_volume"`
	ComicVineIssue  int    `json:"comicvine_issue"`
}

func (p *PhysicalBook) toBook() library.ComicBook {
	b := library.ComicBook{
		ID:                   "{" + uuid.New().String() + "}",
		Series:               strings.TrimSpace(p.Series),
		Number:               strings.TrimSpace(p.Number),
		Volume:               p.Volume,
		Year:                 p.Year,
		Publisher:            strings.TrimSpace(p.Publisher),
		Title:                strings.TrimSpace(p.Title),
		Format:               strings.TrimSpace(p.Format),
		BookLocation:         strings.TrimSpace(p.Location),
		BookCondition:        strings.TrimSpace(p.Condition),
		BookNotes:            strings.TrimSpace(p.Notes),
		BookCollectionStatus: library.CollectionStatusPhysical,
	}
	if p.ComicVineVolume > 0 {
		b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, "comicvine_volume", strconv.Itoa(p.ComicVineVolume))
	}
	if p.ComicVineIssue > 0 {
		b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, "comicvine_issue", strconv.Itoa(p.ComicVineIssue))
	}
	return b
}

// physicalKey identifies a paper copy for duplicate checks: series
// (normalised like the duplicate matcher), volume and number.
func physicalKey(series string, volume int, number string) string {
	return library.CompressedSeriesName(series) + "\x00" + strconv.Itoa(volume) + "\x00" + normalizeIssueNumber(number)
}

func normalizeIssueNumber(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	t := strings.TrimLeft(n, "0")
	if t == "" && n != "" {
		return "0"
	}
	return t
}

// existingPhysicalKeys returns the keys of every paper copy in the library.
func (s *Server) existingPhysicalKeys() (map[string]bool, error) {
	books, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for i := range books {
		if library.IsPhysical(&books[i]) {
			keys[physicalKey(books[i].Series, books[i].Volume, books[i].Number)] = true
		}
	}
	return keys, nil
}

// createPhysical adds one paper copy and audits it.
func (s *Server) createPhysical(p *PhysicalBook) (library.ComicBook, error) {
	book := p.toBook()
	if err := s.backend.CreateBook(&book); err != nil {
		return book, err
	}
	s.backend.MarkDirty(book.ID)
	if s.configDB != nil {
		audit.Record(s.configDB, configdb.AuditRow{
			BookID: book.ID, Action: audit.ActionPhysicalAdd, Series: book.Series, Number: book.Number,
			Changes: `[{"field":"record","old":"","new":"physical copy"}]`,
		})
	}
	return book, nil
}

// handlePhysical adds one paper copy. A copy that already exists (same
// series, volume and number) is refused with 409.
func (s *Server) handlePhysical(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var p PhysicalBook
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(p.Series) == "" {
		http.Error(w, "series is required", http.StatusBadRequest)
		return
	}
	keys, err := s.existingPhysicalKeys()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	if keys[physicalKey(p.Series, p.Volume, p.Number)] {
		http.Error(w, "A paper copy of this issue is already in the library", http.StatusConflict)
		return
	}
	book, err := s.createPhysical(&p)
	if err != nil {
		http.Error(w, "Failed to create book: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.afterPhysicalChange()
	s.writeJSON(w, http.StatusCreated, map[string]string{"id": book.ID})
}

func (s *Server) afterPhysicalChange() {
	s.InvalidateWorkflowCache()
	s.invalidateVolumeSummaries()
}

// PhysicalImportRequest is the body of POST .../physical/import. Without
// Apply nothing is written (preview).
type PhysicalImportRequest struct {
	CSV   string `json:"csv"`
	Apply bool   `json:"apply"`
}

// PhysicalImportRow is one parsed CSV row.
type PhysicalImportRow struct {
	Line   int          `json:"line"`
	Book   PhysicalBook `json:"book"`
	Status string       `json:"status"` // "new", "exists", "error", "added"
	Error  string       `json:"error,omitempty"`
}

// PhysicalImportResult is the import response.
type PhysicalImportResult struct {
	Columns   map[string]string   `json:"columns"` // CSV header -> field it fills
	Ignored   []string            `json:"ignored"` // headers no field uses
	Rows      []PhysicalImportRow `json:"rows"`
	New       int                 `json:"new"`
	Exists    int                 `json:"exists"`
	Errors    int                 `json:"errors"`
	Added     int                 `json:"added"`
	Applied   bool                `json:"applied"`
	Truncated bool                `json:"truncated,omitempty"` // rows beyond maxPhysicalRows were not read
}

const maxPhysicalRows = 10000

// physicalColumns maps a lowercased, space/underscore-free CSV header to the
// field it fills.
var physicalColumns = map[string]string{
	"series": "series", "seriesname": "series", "name": "series",
	"number": "number", "issue": "number", "issuenumber": "number", "#": "number", "no": "number", "num": "number", "issueno": "number", "issuenum": "number",
	"volume": "volume", "vol": "volume",
	"year":      "year",
	"publisher": "publisher",
	"title":     "title",
	"format":    "format",
	"location":  "location", "booklocation": "location", "storage": "location", "storagelocation": "location", "box": "location",
	"condition": "condition", "bookcondition": "condition", "grade": "condition",
	"notes": "notes", "booknotes": "notes",
	"comicvinevolume": "comicvine_volume", "cvvolume": "comicvine_volume", "comicvinevolumeid": "comicvine_volume",
	"comicvineissue": "comicvine_issue", "cvissue": "comicvine_issue", "comicvineissueid": "comicvine_issue", "comicvineid": "comicvine_issue",
}

func normalizeHeader(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "#" {
		return h
	}
	return strings.NewReplacer(" ", "", "_", "", "-", "", ".", "", "#", "").Replace(h)
}

// parsePhysicalCSV reads a header row plus data rows. Headers match
// case-insensitively, ignoring spaces, underscores, dashes and dots.
func parsePhysicalCSV(text string) (PhysicalImportResult, error) {
	res := PhysicalImportResult{Columns: map[string]string{}, Ignored: []string{}, Rows: []PhysicalImportRow{}}
	cr := csv.NewReader(strings.NewReader(text))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return res, fmt.Errorf("could not read the header row: %w", err)
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\uFEFF")
	}
	fields := make([]string, len(header))
	hasSeries := false
	for i, h := range header {
		f, ok := physicalColumns[normalizeHeader(h)]
		if !ok || res.Columns[h] != "" {
			if strings.TrimSpace(h) != "" {
				res.Ignored = append(res.Ignored, h)
			}
			continue
		}
		fields[i] = f
		res.Columns[h] = f
		hasSeries = hasSeries || f == "series"
	}
	if !hasSeries {
		return res, fmt.Errorf("no Series column (headers: %s)", strings.Join(header, ", "))
	}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line, _ := cr.FieldPos(0)
		if err != nil {
			res.Rows = append(res.Rows, PhysicalImportRow{Line: line, Status: "error", Error: err.Error()})
			continue
		}
		if len(res.Rows) >= maxPhysicalRows {
			res.Truncated = true
			break
		}
		row := PhysicalImportRow{Line: line, Status: "new"}
		empty := true
		for i, v := range rec {
			if i >= len(fields) || fields[i] == "" {
				continue
			}
			v = strings.TrimSpace(v)
			if v != "" {
				empty = false
			}
			if msg := setPhysicalField(&row.Book, fields[i], v); msg != "" && row.Error == "" {
				row.Status, row.Error = "error", msg
			}
		}
		if empty {
			continue // blank line
		}
		if row.Error == "" && row.Book.Series == "" {
			row.Status, row.Error = "error", "Series is empty"
		}
		res.Rows = append(res.Rows, row)
	}
	return res, nil
}

// setPhysicalField sets one field from CSV text; it returns a message when
// a number field is not a number.
func setPhysicalField(p *PhysicalBook, field, v string) string {
	num := func(dst *int) string {
		if v == "" {
			return ""
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Sprintf("%s %q is not a number", field, v)
		}
		*dst = n
		return ""
	}
	switch field {
	case "series":
		p.Series = v
	case "number":
		p.Number = strings.TrimPrefix(v, "#")
	case "volume":
		return num(&p.Volume)
	case "year":
		return num(&p.Year)
	case "publisher":
		p.Publisher = v
	case "title":
		p.Title = v
	case "format":
		p.Format = v
	case "location":
		p.Location = v
	case "condition":
		p.Condition = v
	case "notes":
		p.Notes = v
	case "comicvine_volume":
		return num(&p.ComicVineVolume)
	case "comicvine_issue":
		return num(&p.ComicVineIssue)
	}
	return ""
}

// handlePhysicalImport previews or applies a CSV of paper copies. A row
// whose series+volume+number is already a paper copy (in the library or
// earlier in the file) is "exists" and skipped, so re-importing the same
// file adds nothing.
func (s *Server) handlePhysicalImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req PhysicalImportRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	res, err := parsePhysicalCSV(req.CSV)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	keys, err := s.existingPhysicalKeys()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	for i := range res.Rows {
		row := &res.Rows[i]
		if row.Status == "error" {
			res.Errors++
			continue
		}
		k := physicalKey(row.Book.Series, row.Book.Volume, row.Book.Number)
		if keys[k] {
			row.Status = "exists"
			res.Exists++
			continue
		}
		keys[k] = true
		res.New++
		if !req.Apply {
			continue
		}
		if _, err := s.createPhysical(&row.Book); err != nil {
			row.Status, row.Error = "error", err.Error()
			res.Errors++
			continue
		}
		row.Status = "added"
		res.Added++
	}
	res.Applied = req.Apply
	if res.Added > 0 {
		s.afterPhysicalChange()
	}
	s.writeJSON(w, http.StatusOK, res)
}
