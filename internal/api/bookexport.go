package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Export books (tracker #7, ComicRack's Export Books dialog), first step:
// write a CBZ copy of each picked book into server.export_path. The original
// is never changed, moved or deleted, and the library record is not touched.
// POST /api/library/browse/export-books starts a background job;
// GET /api/library/browse/export-books-job reports it.

const (
	exportNamingOriginal = "original" // the source file's name, with .cbz
	exportNamingSeries   = "series"   // "Series V2 #003 (2019).cbz"
	maxExportNameBytes   = 150
	maxExportErrors      = 50
)

// BookExportRequest is the POST body.
type BookExportRequest struct {
	// BookIDs, when not empty, is the scope. Otherwise Browse is.
	BookIDs []string      `json:"book_ids,omitempty"`
	Browse  BrowseRequest `json:"browse"`
	// ExpectedCount guards a Browse scope against a library that changed
	// since the person saw the count.
	ExpectedCount int `json:"expected_count"`
	// Naming is "original" (default) or "series".
	Naming string `json:"naming"`
	// Subfolder is one folder name under export_path to group this run.
	Subfolder string `json:"subfolder"`
	// Overwrite replaces an earlier export with the same name. Off skips it.
	Overwrite bool `json:"overwrite"`

	// Page options (tracker #7, step 2). All empty = pages copied unchanged.
	Format      string `json:"format"`       // "" keep | "jpeg" | "png"
	Quality     int    `json:"quality"`      // JPEG quality 1-100; 0 = 85
	Resize      string `json:"resize"`       // "" | "width" | "height" | "fit"
	Width       int    `json:"width"`        // pixels; used by width and fit
	Height      int    `json:"height"`       // pixels; used by height and fit
	DontEnlarge bool   `json:"dont_enlarge"` // never scale a page up
}

func (r BookExportRequest) pageOptions() cbzconvert.PageOptions {
	return cbzconvert.PageOptions{Format: r.Format, Quality: r.Quality, Resize: r.Resize, Width: r.Width, Height: r.Height, DontEnlarge: r.DontEnlarge}
}

// BookExportJob is the state of the one export job (running or last finished).
type BookExportJob struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"` // "running" or "completed"
	Folder      string     `json:"folder"`
	Total       int        `json:"total"`
	Processed   int        `json:"processed"`
	Exported    int        `json:"exported"`
	Skipped     int        `json:"skipped"` // target already there, or nothing to export
	Failed      int        `json:"failed"`
	Errors      []string   `json:"errors,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// exportFileName builds the target file name (with .cbz) for book.
func exportFileName(book *library.ComicBook, naming string) string {
	var base string
	if naming == exportNamingSeries && strings.TrimSpace(book.Series) != "" {
		parts := []string{strings.TrimSpace(book.Series)}
		if book.Volume > 0 {
			parts = append(parts, "V"+strconv.Itoa(book.Volume))
		}
		if n := strings.TrimSpace(book.Number); n != "" {
			if v, err := strconv.Atoi(n); err == nil && v >= 0 && v < 1000 {
				n = fmt.Sprintf("%03d", v)
			}
			parts = append(parts, "#"+n)
		}
		if book.Year > 0 {
			parts = append(parts, "("+strconv.Itoa(book.Year)+")")
		}
		base = strings.Join(parts, " ")
	} else {
		p := strings.ReplaceAll(book.FilePath, "\\", "/")
		base = strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	}
	base = sanitizeExportName(base)
	if base == "" {
		base = sanitizeExportName(book.ID)
	}
	if base == "" {
		base = "book"
	}
	for len(base) > maxExportNameBytes {
		_, size := lastRune(base)
		base = base[:len(base)-size]
	}
	return strings.TrimRight(base, ". ") + ".cbz"
}

func lastRune(s string) (rune, int) {
	r := []rune(s)
	last := r[len(r)-1]
	return last, len(string(last))
}

// sanitizeExportName replaces characters that are unsafe in a file name on
// Linux or Windows, and trims dots and spaces from the ends.
func sanitizeExportName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f || unicode.IsControl(r):
			b.WriteRune('_')
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ". ")
}

// exportFolder returns root, or root/subfolder, refusing anything that would
// leave root.
func exportFolder(root, subfolder string) (string, error) {
	subfolder = strings.TrimSpace(subfolder)
	if subfolder == "" {
		return filepath.Clean(root), nil
	}
	if strings.ContainsAny(subfolder, `/\`) || subfolder == "." || subfolder == ".." {
		return "", errors.New("The subfolder must be one folder name, with no slashes")
	}
	clean := sanitizeExportName(subfolder)
	if clean == "" {
		return "", errors.New("The subfolder name is not usable")
	}
	return filepath.Join(filepath.Clean(root), clean), nil
}

// effectiveExportPath returns the export folder in effect: the one saved in
// Settings (config.db) if set, else the config file / env value (tracker #48).
func (s *Server) effectiveExportPath() string {
	if s.configDB != nil {
		if st, err := s.configDB.GetServerMiscSettings(); err == nil && st != nil && st.ExportPath != "" {
			return st.ExportPath
		}
	}
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	if s.config == nil {
		return ""
	}
	return s.config.Server.ExportPath
}

// validateExportPath checks a folder before it is saved as the export folder:
// absolute, creatable, writable, and not inside the library root (an export
// must never land among the originals).
func validateExportPath(path, libraryRoot string) error {
	if !filepath.IsAbs(path) {
		return errors.New("use a full path, starting with /")
	}
	clean := filepath.Clean(path)
	if libraryRoot != "" && pathWithin(clean, filepath.Clean(libraryRoot)) {
		return errors.New("it is inside the library folder; pick a folder outside it")
	}
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return fmt.Errorf("the server cannot create it: %w", err)
	}
	probe, err := os.CreateTemp(clean, ".export-check-*")
	if err != nil {
		return fmt.Errorf("the server cannot write to it: %w", err)
	}
	probe.Close()
	os.Remove(probe.Name()) // our own empty probe file
	return nil
}

// pathWithin reports whether child is parent or lies under it.
func pathWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// GET /api/library/browse/export-books-config tells the Browse panel whether
// an export folder is set.
func (s *Server) handleBookExportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"configured": s.effectiveExportPath() != ""})
}

// POST /api/library/browse/export-books
func (s *Server) handleBookExportStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	root := s.effectiveExportPath()
	s.configMu.RLock()
	embed := s.config != nil && s.config.Server.CBZConvert.EmbedComicBook
	s.configMu.RUnlock()
	if root == "" {
		http.Error(w, "Export is off: set the export folder in Settings", http.StatusServiceUnavailable)
		return
	}

	var req BookExportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Naming == "" {
		req.Naming = exportNamingOriginal
	}
	if req.Naming != exportNamingOriginal && req.Naming != exportNamingSeries {
		http.Error(w, "naming must be \"original\" or \"series\"", http.StatusBadRequest)
		return
	}
	if err := req.pageOptions().Validate(); err != nil {
		http.Error(w, "Page options: "+err.Error(), http.StatusBadRequest)
		return
	}
	folder, err := exportFolder(root, req.Subfolder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ids, err := s.bulkEditScope(BulkEditRequest{BookIDs: req.BookIDs, Browse: req.Browse})
	if err != nil {
		log.Error().Err(err).Msg("Failed to resolve the export scope")
		http.Error(w, "Failed to resolve the selection", http.StatusInternalServerError)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "No books selected", http.StatusBadRequest)
		return
	}
	if len(ids) > maxBulkEditBooks {
		http.Error(w, fmt.Sprintf("%d books selected; the limit is %d. Narrow the filter.", len(ids), maxBulkEditBooks), http.StatusRequestEntityTooLarge)
		return
	}
	if len(req.BookIDs) == 0 && req.ExpectedCount != len(ids) {
		http.Error(w, fmt.Sprintf("The selection now holds %d books, not the %d you saw. Try again.", len(ids), req.ExpectedCount), http.StatusConflict)
		return
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		log.Error().Err(err).Str("folder", folder).Msg("Failed to create the export folder")
		http.Error(w, "Could not create the export folder", http.StatusInternalServerError)
		return
	}

	s.exportJobMu.Lock()
	if s.exportJob != nil && s.exportJob.Status == "running" {
		s.exportJobMu.Unlock()
		http.Error(w, "An export is already running", http.StatusConflict)
		return
	}
	job := &BookExportJob{JobID: uuid.New().String(), Status: "running", Folder: folder, Total: len(ids), StartedAt: time.Now()}
	s.exportJob = job
	s.exportJobMu.Unlock()

	go s.runBookExport(job, ids, folder, req, embed)
	s.writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.JobID})
}

// GET /api/library/browse/export-books-job
func (s *Server) handleBookExportStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.exportJobMu.RLock()
	defer s.exportJobMu.RUnlock()
	if s.exportJob == nil {
		http.Error(w, "No export has run", http.StatusNotFound)
		return
	}
	copied := *s.exportJob
	copied.Errors = append([]string(nil), s.exportJob.Errors...)
	s.writeJSON(w, http.StatusOK, copied)
}

func (s *Server) runBookExport(job *BookExportJob, ids []string, folder string, req BookExportRequest, embed bool) {
	used := map[string]bool{}
	opts := cbzconvert.Options{EmbedComicBook: embed}
	for _, id := range ids {
		outcome, err := s.exportOneBook(id, folder, req, opts, used)
		s.exportJobMu.Lock()
		job.Processed++
		switch {
		case err != nil:
			job.Failed++
			if len(job.Errors) < maxExportErrors {
				job.Errors = append(job.Errors, fmt.Sprintf("%s: %v", id, err))
			}
		case outcome == "exported":
			job.Exported++
		default:
			job.Skipped++
		}
		s.exportJobMu.Unlock()
		if err != nil {
			log.Warn().Err(err).Str("book_id", id).Msg("Export failed for book")
		}
	}
	now := time.Now()
	s.exportJobMu.Lock()
	job.Status = "completed"
	job.CompletedAt = &now
	s.exportJobMu.Unlock()
	log.Info().Int("exported", job.Exported).Int("skipped", job.Skipped).Int("failed", job.Failed).Msg("Book export finished")
}

// exportOneBook returns "exported" or "skipped". A book that vanished or has
// no archive is skipped; a real failure is an error.
func (s *Server) exportOneBook(id, folder string, req BookExportRequest, opts cbzconvert.Options, used map[string]bool) (string, error) {
	b, err := s.backend.GetBook(id)
	if err != nil {
		return "", err
	}
	if b == nil {
		return "skipped", nil
	}
	book := *b // the XML backend hands out a pointer into the live library
	if book.FilePath == "" || cbzconvert.IsWebComicPointer(book.FilePath) {
		return "skipped", nil
	}
	src := s.resolveBookFilePath(book.FilePath)
	if pathWithin(filepath.Dir(filepath.Clean(src)), filepath.Clean(folder)) {
		return "", errors.New("the export folder holds this book's original; pick another folder")
	}
	name := exportFileName(&book, req.Naming)
	if used[strings.ToLower(name)] {
		stem := strings.TrimSuffix(name, ".cbz")
		for n := 2; ; n++ {
			name = fmt.Sprintf("%s (%d).cbz", stem, n)
			if !used[strings.ToLower(name)] {
				break
			}
		}
	}
	used[strings.ToLower(name)] = true

	_, err = cbzconvert.ExportCopy(&book, src, filepath.Join(folder, name), opts, req.pageOptions(), req.Overwrite)
	if errors.Is(err, cbzconvert.ErrExportExists) {
		return "skipped", nil
	}
	if err != nil {
		return "", err
	}
	return "exported", nil
}
