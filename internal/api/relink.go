package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/relink"
)

// Relink and Remove for books whose file is missing (comic-server-heox).
// Relink only looks inside the book's old folder (one directory listing,
// never a library walk - the disks are shared with media). A manual folder
// browse for the rest is the manual pick (comic-server-679r): the request's
// file_path names one file, checked here and never searched for.

// Relink statuses.
const (
	relinkFound         = "found"          // a single fitting file in the old folder
	relinkNotFound      = "not_found"      // nothing fits
	relinkAmbiguous     = "ambiguous"      // several fit; none is picked
	relinkFolderMissing = "folder_missing" // the old folder is gone too
	relinkNotMissing    = "not_missing"    // the book's file is on disk
	relinkNoFile        = "no_file"        // wanted/paper copy: nothing to relink
)

// RelinkOutcome is one book's answer. NewPath is in the library's own path
// style (what FilePath would hold), not the resolved on-disk path.
type RelinkOutcome struct {
	BookID     string   `json:"book_id"`
	Series     string   `json:"series,omitempty"`
	Number     string   `json:"number,omitempty"`
	Status     string   `json:"status"`
	OldPath    string   `json:"old_path,omitempty"`
	NewPath    string   `json:"new_path,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
	Applied    bool     `json:"applied,omitempty"`
	// StartDir is where the manual file picker should open: the book's old
	// folder, or its nearest parent that still exists. Empty = no good guess.
	StartDir string `json:"start_dir,omitempty"`
}

// nearestExistingDir walks up from dir to the first folder that exists. It
// returns "" when nothing but the disk root (or no absolute path) is left,
// so the caller keeps its own default.
func nearestExistingDir(dir string) string {
	for d := filepath.Clean(dir); filepath.IsAbs(d) && d != filepath.Dir(d); d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return d
		}
	}
	return ""
}

// relinkContext carries what one run shares between books: which paths other
// books use, and each folder's listing (read at most once per run).
type relinkContext struct {
	s        *Server
	used     map[string]string // cleaned resolved path -> book id
	listings map[string][]string
}

func (s *Server) newRelinkContext() (*relinkContext, error) {
	all, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	used := make(map[string]string, len(all))
	for i := range all {
		if p := all[i].FilePath; p != "" {
			used[filepath.Clean(s.resolveBookFilePath(p))] = all[i].ID
		}
	}
	return &relinkContext{s: s, used: used, listings: map[string][]string{}}, nil
}

// listDir returns the regular-file names in dir, cached per run.
func (c *relinkContext) listDir(dir string) ([]string, error) {
	if names, ok := c.listings[dir]; ok {
		return names, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	c.listings[dir] = names
	return names, nil
}

// find answers for one book without changing anything.
func (c *relinkContext) find(book *library.ComicBook) RelinkOutcome {
	out := RelinkOutcome{BookID: book.ID, Series: book.Series, Number: book.Number, OldPath: book.FilePath}
	if book.FilePath == "" {
		out.Status = relinkNoFile
		return out
	}
	resolved := c.s.resolveBookFilePath(book.FilePath)
	if _, err := os.Stat(resolved); err == nil {
		out.Status = relinkNotMissing
		return out
	}
	dir := filepath.Dir(resolved)
	out.StartDir = nearestExistingDir(dir)
	names, err := c.listDir(dir)
	if err != nil {
		out.Status = relinkFolderMissing
		return out
	}
	rawDir, oldName := relink.SplitRawPath(book.FilePath)
	taken := func(name string) bool {
		id, ok := c.used[filepath.Clean(filepath.Join(dir, name))]
		return ok && id != book.ID
	}
	res := relink.Find(oldName, book, names, taken)
	switch res.Outcome {
	case relink.Found:
		out.Status = relinkFound
		out.NewPath = rawDir + res.Match
		out.Reason = res.Reason
	case relink.Ambiguous:
		out.Status = relinkAmbiguous
		out.Candidates = res.Candidates
	default:
		out.Status = relinkNotFound
	}
	return out
}

// forgetMissing drops books from the disk-check result so the flag clears
// now instead of at the next background pass.
func (s *Server) forgetMissing(ids ...string) {
	s.disk.mu.Lock()
	for _, id := range ids {
		delete(s.disk.missing, id)
	}
	s.disk.mu.Unlock()
	s.InvalidateWorkflowCache()
}

type relinkRequest struct {
	Apply bool `json:"apply"` // false = preview only
	// FilePath is a file the user picked by hand (comic-server-679r), as a
	// path on this server's disk. Empty = search the old folder.
	FilePath string `json:"file_path,omitempty"`
}

// Extra statuses for a manual pick.
const (
	relinkPickBad   = "pick_invalid" // not a file, not a comic, or outside the library root
	relinkPickTaken = "pick_taken"   // another book already uses that file
)

// rawPathForPick turns a picked on-disk path into the library's own path
// style. With a library root pair set, the pick must sit under library_root
// and is written under library_source_root, keeping the separator style of
// the book's old path. With no pair, the pick is stored as it is.
func (s *Server) rawPathForPick(picked, oldRaw string) (string, bool) {
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()
	if cfg == nil || cfg.Server.LibrarySourceRoot == "" || cfg.Server.LibraryRoot == "" {
		return picked, true
	}
	root := filepath.Clean(cfg.Server.LibraryRoot)
	rel, err := filepath.Rel(root, picked)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	sep := "/"
	if strings.Contains(oldRaw, `\`) && !strings.Contains(oldRaw, "/") {
		sep = `\`
	}
	src := strings.TrimRight(cfg.Server.LibrarySourceRoot, `/\`)
	return src + sep + strings.ReplaceAll(filepath.ToSlash(rel), "/", sep), true
}

// pick answers for a hand-picked file without changing anything. Same safety
// as the folder search: the file must exist, be a comic archive, and not be
// used by another book.
func (c *relinkContext) pick(book *library.ComicBook, picked string) RelinkOutcome {
	out := RelinkOutcome{BookID: book.ID, Series: book.Series, Number: book.Number, OldPath: book.FilePath}
	if book.FilePath == "" {
		out.Status = relinkNoFile
		return out
	}
	if _, err := os.Stat(c.s.resolveBookFilePath(book.FilePath)); err == nil {
		out.Status = relinkNotMissing
		return out
	}
	picked = filepath.Clean(picked)
	fi, err := os.Stat(picked)
	if err != nil || !fi.Mode().IsRegular() {
		out.Status, out.Reason = relinkPickBad, "That is not a file on the server."
		return out
	}
	if !relink.IsComicFile(picked) {
		out.Status, out.Reason = relinkPickBad, "That is not a comic archive."
		return out
	}
	raw, ok := c.s.rawPathForPick(picked, book.FilePath)
	if !ok {
		out.Status, out.Reason = relinkPickBad, "That file is outside the library root."
		return out
	}
	if id, used := c.used[filepath.Clean(c.s.resolveBookFilePath(raw))]; used && id != book.ID {
		out.Status, out.Reason = relinkPickTaken, "Another book already uses that file."
		return out
	}
	out.Status = relinkFound
	out.NewPath = raw
	out.Reason = "picked by hand"
	return out
}

// handleBookRelink previews or applies a relink for one book.
// POST /api/library/books/:id/relink {apply}
func (s *Server) handleBookRelink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req relinkRequest
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req)

	backend := s.audited(audit.ActionRelink)
	book, err := backend.GetBook(bookDetailID(r.URL.Path, "/relink"))
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	ctx, err := s.newRelinkContext()
	if err != nil {
		http.Error(w, "Failed to load library", http.StatusInternalServerError)
		return
	}
	var out RelinkOutcome
	if req.FilePath != "" {
		out = ctx.pick(book, req.FilePath)
	} else {
		out = ctx.find(book)
	}
	if req.Apply && out.Status == relinkFound {
		if err := s.applyRelink(backend, []*library.ComicBook{book}, []RelinkOutcome{out}); err != nil {
			log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save relink")
			http.Error(w, "Failed to save book", http.StatusInternalServerError)
			return
		}
		out.Applied = true
	}
	s.writeJSON(w, http.StatusOK, out)
}

// applyRelink sets each book's FilePath (and the file size, read from the
// found file) and saves them in one batch. books[i] pairs with outs[i].
func (s *Server) applyRelink(backend library.Backend, books []*library.ComicBook, outs []RelinkOutcome) error {
	for i, b := range books {
		b.FilePath = outs[i].NewPath
		if fi, err := os.Stat(s.resolveBookFilePath(b.FilePath)); err == nil {
			b.FileSize = fi.Size()
		}
	}
	if err := backend.UpdateBooks(books); err != nil {
		return err
	}
	ids := make([]string, len(books))
	for i, b := range books {
		ids[i] = b.ID
	}
	s.forgetMissing(ids...)
	return nil
}

// handleBookRemoveRecord deletes only the library record of a book whose
// file is gone. It never touches a file and refuses if the file is back.
// POST /api/library/books/:id/remove-record
func (s *Server) handleBookRemoveRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	id := bookDetailID(r.URL.Path, "/remove-record")
	book, err := s.backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if book.FilePath == "" {
		http.Error(w, "This book has no file; only books with a missing file can be removed here", http.StatusConflict)
		return
	}
	if _, err := os.Stat(s.resolveBookFilePath(book.FilePath)); err == nil {
		http.Error(w, "The file is on disk again; the record was not removed", http.StatusConflict)
		return
	}
	snapshot := *book
	if err := s.backend.DeleteBook(id); err != nil {
		log.Error().Err(err).Str("book_id", id).Msg("Failed to remove book record")
		http.Error(w, "Failed to remove the record", http.StatusInternalServerError)
		return
	}
	if s.configDB != nil {
		changes, _ := json.Marshal([]audit.Change{{Field: "record", Old: snapshot.FilePath, New: "(removed; file was missing)"}})
		audit.Record(s.configDB, configdb.AuditRow{
			BookID: snapshot.ID, Action: audit.ActionRemoveRecord, Series: snapshot.Series, Number: snapshot.Number,
			Changes: string(changes),
		})
	}
	s.forgetMissing(id)
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "removed": true})
}

// RelinkMissingResponse is the bulk answer.
type RelinkMissingResponse struct {
	Applied bool            `json:"applied"`
	Total   int             `json:"total"` // missing books looked at
	Counts  map[string]int  `json:"counts"`
	Rows    []RelinkOutcome `json:"rows"` // found + ambiguous first, capped
}

const relinkRowCap = 200

// handleRelinkMissing previews or applies relink for every book carrying the
// file_missing flag (or only book_ids). One directory listing per folder.
// POST /api/library/workflow/relink-missing {apply, book_ids?}
func (s *Server) handleRelinkMissing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Apply   bool     `json:"apply"`
		BookIDs []string `json:"book_ids"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)

	entry, err := s.getOrBuildWorkflowCache()
	if err != nil {
		http.Error(w, "Failed to load books", http.StatusInternalServerError)
		return
	}
	missing := entry.booksByFlag[attention.FlagFileMissing]
	if len(req.BookIDs) > 0 {
		want := map[string]bool{}
		for _, id := range req.BookIDs {
			want[id] = true
		}
		var filtered []*library.ComicBook
		for _, b := range missing {
			if want[b.ID] {
				filtered = append(filtered, b)
			}
		}
		missing = filtered
	}

	ctx, err := s.newRelinkContext()
	if err != nil {
		http.Error(w, "Failed to load library", http.StatusInternalServerError)
		return
	}
	resp := RelinkMissingResponse{Applied: req.Apply, Total: len(missing), Counts: map[string]int{}}
	var rows []RelinkOutcome
	var toBooks []*library.ComicBook
	var toOuts []RelinkOutcome
	for _, b := range missing {
		out := ctx.find(b)
		resp.Counts[out.Status]++
		rows = append(rows, out)
		if out.Status == relinkFound {
			// A target two missing books would both claim is not safe.
			if prior, dup := ctx.used[filepath.Clean(s.resolveBookFilePath(out.NewPath))]; dup && prior != b.ID {
				resp.Counts[relinkFound]--
				resp.Counts[relinkAmbiguous]++
				rows[len(rows)-1].Status = relinkAmbiguous
				rows[len(rows)-1].NewPath = ""
				continue
			}
			ctx.used[filepath.Clean(s.resolveBookFilePath(out.NewPath))] = b.ID
			toBooks = append(toBooks, b)
			toOuts = append(toOuts, out)
		}
	}

	if req.Apply && len(toBooks) > 0 {
		backend := s.audited(audit.ActionRelink)
		// Work on copies read through the audited wrapper so the diff is real.
		working := make([]*library.ComicBook, 0, len(toBooks))
		for _, b := range toBooks {
			cp, err := backend.GetBook(b.ID)
			if err != nil || cp == nil {
				http.Error(w, "Book not found: "+b.ID, http.StatusInternalServerError)
				return
			}
			working = append(working, cp)
		}
		if err := s.applyRelink(backend, working, toOuts); err != nil {
			log.Error().Err(err).Msg("Failed to save relinks")
			http.Error(w, fmt.Sprintf("Failed to save: %v", err), http.StatusInternalServerError)
			return
		}
		applied := map[string]bool{}
		for _, o := range toOuts {
			applied[o.BookID] = true
		}
		for i := range rows {
			rows[i].Applied = applied[rows[i].BookID]
		}
	}

	rank := map[string]int{relinkFound: 0, relinkAmbiguous: 1}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, oki := rank[rows[i].Status]
		rj, okj := rank[rows[j].Status]
		if !oki {
			ri = 2
		}
		if !okj {
			rj = 2
		}
		return ri < rj
	})
	if len(rows) > relinkRowCap {
		rows = rows[:relinkRowCap]
	}
	resp.Rows = rows
	s.writeJSON(w, http.StatusOK, resp)
}
