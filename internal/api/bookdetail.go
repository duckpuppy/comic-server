package api

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/attention"
	"github.com/duckpuppy/comic-server/internal/cbzconvert"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// Book detail page (comic-server-br14), view-only.
//
// GET /api/library/books/:id            the record plus derived state
// GET /api/library/books/:id/comicinfo  the file's ComicInfo.xml vs the library

// BookDetailFlag is one attention flag on the book.
type BookDetailFlag struct {
	Flag     string `json:"flag"`
	Label    string `json:"label"`
	Hint     string `json:"hint"`
	Severity string `json:"severity"`
}

// BookDetailFile is what is on disk for the book.
type BookDetailFile struct {
	Path         string `json:"path"`          // raw library path
	ResolvedPath string `json:"resolved_path"` // path this server opens
	Exists       bool   `json:"exists"`
	Size         int64  `json:"size,omitempty"`
	Ext          string `json:"ext,omitempty"`
}

// BookDetailComicVine holds the book's ComicVine IDs and links.
type BookDetailComicVine struct {
	VolumeID  int    `json:"volume_id,omitempty"`
	IssueID   int    `json:"issue_id,omitempty"`
	VolumeURL string `json:"volume_url,omitempty"`
	IssueURL  string `json:"issue_url,omitempty"`
}

// BookDetail is the response of GET /api/library/books/:id.
type BookDetail struct {
	Book         library.ComicBook    `json:"book"`
	CustomValues map[string]string    `json:"custom_values"`
	Stage        string               `json:"stage,omitempty"`
	StageLabel   string               `json:"stage_label,omitempty"`
	Flags        []BookDetailFlag     `json:"flags"`
	File         *BookDetailFile      `json:"file,omitempty"` // nil for a fileless book
	ComicVine    *BookDetailComicVine `json:"comicvine,omitempty"`
	CoverURL     string               `json:"cover_url"`
	Actions      BookDetailActions    `json:"actions"`
}

// BookDetailStage is one stage a book can be rolled back to.
type BookDetailStage struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// BookDetailActions says which detail-page actions can run for this book
// (comic-server-2s4f). A *Blocker string is "" when the action can run,
// otherwise the reason it cannot.
type BookDetailActions struct {
	WriteMetadataBlocker string            `json:"write_metadata_blocker"`
	Stale                bool              `json:"stale"`
	ScrapeBlocker        string            `json:"scrape_blocker"`
	VariantCoversBlocker string            `json:"variant_covers_blocker"`
	RollbackStages       []BookDetailStage `json:"rollback_stages"`
}

// ComicInfoField is one ComicInfo.xml element that differs between the
// file and what the library would write.
type ComicInfoField struct {
	Field   string `json:"field"`
	File    string `json:"file"`
	Library string `json:"library"`
}

// ComicInfoComparison is the response of GET .../comicinfo.
type ComicInfoComparison struct {
	Present   bool             `json:"present"`             // file has a ComicInfo.xml
	Raw       string           `json:"raw,omitempty"`       // the file's XML, capped
	Truncated bool             `json:"truncated,omitempty"` // Raw was cut at maxComicInfoRaw
	Diff      []ComicInfoField `json:"diff"`
	Same      int              `json:"same"` // fields that agree
	Error     string           `json:"error,omitempty"`
}

const maxComicInfoRaw = 256 << 10

// bookDetailID extracts the book ID from /api/library/books/:id[/suffix].
func bookDetailID(path, suffix string) string {
	id := strings.TrimPrefix(path, "/api/library/books/")
	id = strings.TrimSuffix(id, suffix)
	id = strings.TrimSuffix(id, "/")
	if strings.Contains(id, "/") {
		return ""
	}
	return id
}

// loadBookCopy returns a copy of the book, so a live XML-backend pointer is
// never read while something else writes it.
func (s *Server) loadBookCopy(w http.ResponseWriter, r *http.Request, suffix string) (*library.ComicBook, bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return nil, false
	}
	id := bookDetailID(r.URL.Path, suffix)
	if id == "" {
		http.NotFound(w, r)
		return nil, false
	}
	b, err := s.backend.GetBook(id)
	if err != nil || b == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return nil, false
	}
	cp := *b
	cp.Pages = append([]library.ComicPageInfo(nil), b.Pages...)
	return &cp, true
}

// handleBookDetail GET /api/library/books/:id
func (s *Server) handleBookDetail(w http.ResponseWriter, r *http.Request) {
	book, ok := s.loadBookCopy(w, r, "")
	if !ok {
		return
	}
	d := BookDetail{
		Book:         *book,
		CustomValues: library.CustomValuesMap(book.CustomValuesStore),
		Flags:        []BookDetailFlag{},
		CoverURL:     "/api/library/books/" + book.ID + "/cover",
	}
	if st := workflow.GetStage(book); st != workflow.StageUnknown {
		d.Stage, d.StageLabel = st.String(), st.Label()
	}

	// Flags come from the workflow cache, so they match the Needs attention
	// panel exactly (including file_missing and the ComicVine checks).
	if entry, err := s.getOrBuildWorkflowCache(); err == nil {
		for f, books := range entry.booksByFlag {
			for _, b := range books {
				if b.ID != book.ID {
					continue
				}
				if info, ok := attention.Lookup(f); ok {
					d.Flags = append(d.Flags, BookDetailFlag{string(f), info.Label, info.Hint, info.Severity.String()})
				}
				break
			}
		}
		sort.Slice(d.Flags, func(i, j int) bool { return attentionRank(d.Flags[i]) < attentionRank(d.Flags[j]) })
	}

	if book.FilePath != "" {
		f := &BookDetailFile{Path: book.FilePath, ResolvedPath: s.resolveBookFilePath(book.FilePath)}
		f.Ext = strings.ToLower(filepath.Ext(book.FilePath))
		if st, err := os.Stat(f.ResolvedPath); err == nil && !st.IsDir() {
			f.Exists, f.Size = true, st.Size()
		}
		d.File = f
	}

	vol, iss := comicvine.VolumeIDFromStore(book.CustomValuesStore), comicvine.IssueIDFromStore(book.CustomValuesStore)
	if vol > 0 || iss > 0 {
		cv := &BookDetailComicVine{VolumeID: vol, IssueID: iss}
		if vol > 0 {
			cv.VolumeURL = "https://comicvine.gamespot.com/volume/4050-" + strconv.Itoa(vol) + "/"
		}
		if iss > 0 {
			cv.IssueURL = "https://comicvine.gamespot.com/issue/4000-" + strconv.Itoa(iss) + "/"
		}
		d.ComicVine = cv
	}
	d.Actions = s.bookDetailActions(book, iss)
	s.writeJSON(w, http.StatusOK, d)
}

func (s *Server) bookDetailActions(book *library.ComicBook, issueID int) BookDetailActions {
	a := BookDetailActions{RollbackStages: []BookDetailStage{}}
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()

	if book.FilePath == "" {
		a.WriteMetadataBlocker = "this book has no file"
		a.VariantCoversBlocker = "this book has no file"
	} else {
		a.WriteMetadataBlocker = s.variantApplyBlocker(book)
		a.VariantCoversBlocker = a.WriteMetadataBlocker
		if cfg != nil {
			a.Stale = cbzconvert.IsStale(book, cfg.Server.CBZConvert.EmbedComicBook)
		}
	}
	switch {
	case a.VariantCoversBlocker != "":
	case s.cvClient == nil:
		a.VariantCoversBlocker = "no ComicVine API key is configured"
	case issueID == 0:
		a.VariantCoversBlocker = "the book has no ComicVine issue tag; scrape it first"
	}

	switch {
	case s.scraper == nil:
		a.ScrapeBlocker = "the ComicVine scraper is not configured"
	case book.FilePath == "":
		a.ScrapeBlocker = "the scraper matches by file name; this book has no file"
	}

	// Rollback only moves backwards, and needs the audit log (config.db).
	if cur := workflow.GetStage(book); cur != workflow.StageUnknown && s.configDB != nil {
		for _, st := range workflow.Stages {
			if st >= cur {
				break
			}
			a.RollbackStages = append(a.RollbackStages, BookDetailStage{st.String(), st.Label()})
		}
	}
	return a
}

// attentionRank orders flags like the Needs attention panel: severity
// first, then catalog order.
func attentionRank(f BookDetailFlag) int {
	sev := map[string]int{"error": 0, "warn": 1, "info": 2}[f.Severity]
	for i, info := range attention.Catalog {
		if string(info.Flag) == f.Flag {
			return sev*1000 + i
		}
	}
	return sev*1000 + 999
}

// handleBookComicInfo GET /api/library/books/:id/comicinfo
//
// Compares the file's embedded ComicInfo.xml with what comic-server would
// write for the book now (cbzconvert.BuildComicInfoXML), element by
// element. The Pages block is skipped: page entries are file layout, not
// metadata. Opens the archive, so it is only called on demand.
func (s *Server) handleBookComicInfo(w http.ResponseWriter, r *http.Request) {
	book, ok := s.loadBookCopy(w, r, "/comicinfo")
	if !ok {
		return
	}
	res := ComicInfoComparison{Diff: []ComicInfoField{}}
	if book.FilePath == "" {
		res.Error = "This book has no file."
		s.writeJSON(w, http.StatusOK, res)
		return
	}
	raw, present := comicvine.ReadComicInfoXMLBytes(s.resolveBookFilePath(book.FilePath))
	res.Present = present
	if len(raw) > maxComicInfoRaw {
		res.Raw, res.Truncated = string(raw[:maxComicInfoRaw]), true
	} else {
		res.Raw = string(raw)
	}

	want, err := cbzconvert.BuildComicInfoXML(book, book.PageCount)
	if err != nil {
		res.Error = "Could not build the library's ComicInfo.xml: " + err.Error()
		s.writeJSON(w, http.StatusOK, res)
		return
	}
	fileFields, err := flattenComicInfo(raw)
	if err != nil && present {
		res.Error = "The file's ComicInfo.xml is not valid XML: " + err.Error()
	}
	libFields, _ := flattenComicInfo(want)
	keys := map[string]bool{}
	for k := range fileFields {
		keys[k] = true
	}
	for k := range libFields {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if fileFields[k] == libFields[k] {
			res.Same++
			continue
		}
		res.Diff = append(res.Diff, ComicInfoField{Field: k, File: fileFields[k], Library: libFields[k]})
	}
	s.writeJSON(w, http.StatusOK, res)
}

// flattenComicInfo returns the root element's direct children as
// name -> trimmed text, skipping Pages. Empty values are dropped so an
// absent element and an empty one compare equal.
func flattenComicInfo(data []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(data) == 0 {
		return out, nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	var name string
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 {
				name = t.Name.Local
				text.Reset()
			}
		case xml.CharData:
			if depth == 2 {
				text.Write(t)
			}
		case xml.EndElement:
			if depth == 2 && name != "Pages" {
				if v := strings.TrimSpace(text.String()); v != "" {
					out[name] = v
				}
			}
			depth--
		}
	}
}
