package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/collects"
	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Suggested collects links (tracker #40). A book's description often says
// "Collects GHOST RIDER (2006) #33-35". The text is parsed (collects.ParseSummary)
// and each title is looked up among the volumes already in the local ComicVine
// cache (comicvine.VolumeIndex). Nothing is fetched from ComicVine and nothing
// is saved until a person accepts: suggestions are computed on demand, and the
// only thing stored is the "dismissed" mark.

const collectsSuggestTTL = 2 * time.Minute

// CollectsSuggestionEntry is one proposed link and why.
type CollectsSuggestionEntry struct {
	collects.Entry
	Text       string   `json:"text"`                  // what the description said
	VolumeName string   `json:"volume_name,omitempty"` // the cached ComicVine volume, when resolved
	VolumeYear string   `json:"volume_year,omitempty"`
	Confidence string   `json:"confidence"` // high, medium, low
	Reason     string   `json:"reason"`
	Warnings   []string `json:"warnings,omitempty"`
}

// CollectsSuggestion is a book with its proposed links.
type CollectsSuggestion struct {
	BookID  string                    `json:"book_id"`
	Series  string                    `json:"series"`
	Number  string                    `json:"number"`
	Title   string                    `json:"title"`
	Format  string                    `json:"format"`
	Excerpt string                    `json:"excerpt"`
	Entries []CollectsSuggestionEntry `json:"entries"`
	Notes   []string                  `json:"notes,omitempty"`
	score   int                       // sum over entries: high 2, medium 1; the list sort key
	best    int                       // best single entry, same scale; the "min" filter
}

// excerptAround returns up to 300 characters of the summary from the
// "collect..." statement on.
func excerptAround(summary string) string {
	i := strings.Index(strings.ToLower(summary), "collect")
	if i < 0 {
		i = 0
	}
	r := []rune(summary[i:])
	if len(r) > 300 {
		return string(r[:300]) + "…"
	}
	return string(r)
}

// suggestCollects builds the proposal for one book, or nil when there is
// nothing to propose (links already set, dismissed, a placeholder, no list).
func suggestCollects(b *library.ComicBook, idx *comicvine.VolumeIndex, cache *comicvine.Cache) *CollectsSuggestion {
	if b.FilePath == "" || collects.Has(b) || collects.Dismissed(b) || !strings.Contains(strings.ToLower(b.Summary), "collect") {
		return nil
	}
	parsed := collects.ParseSummary(b.Summary)
	sug := &CollectsSuggestion{
		BookID: b.ID, Series: b.Series, Number: b.Number, Title: b.Title, Format: b.Format,
		Excerpt: excerptAround(b.Summary), Notes: parsed.Notes,
	}
	for _, c := range parsed.Candidates {
		if c.Title == "" {
			continue // a range with no title cannot become a link
		}
		res := idx.Resolve(c.Title, c.Year)
		e := collects.Entry{Series: c.Title, From: c.From, To: c.To, Kind: collects.KindMain, Source: collects.SourceSuggested}
		out := CollectsSuggestionEntry{Text: candidateText(c), Confidence: res.Confidence, Reason: res.Reason}
		if res.Volume != nil {
			e.Volume, e.Series = res.Volume.CVID, res.Volume.Name
			out.VolumeName, out.VolumeYear = res.Volume.Name, res.Volume.StartYear
		}
		var nums []string
		if e.Volume > 0 && cache != nil {
			nums, _ = cache.CachedIssueNumbers(e.Volume)
		}
		out.Warnings = collects.Warnings(e, nums)
		if len(out.Warnings) > 0 && out.Confidence == comicvine.ResolveHigh {
			out.Confidence = comicvine.ResolveMedium
		}
		out.Entry = e.Normalize()
		sug.Entries = append(sug.Entries, out)
		pts := 0
		switch out.Confidence {
		case comicvine.ResolveHigh:
			pts = 2
		case comicvine.ResolveMedium:
			pts = 1
		}
		sug.score += pts
		sug.best = max(sug.best, pts)
	}
	if len(sug.Entries) == 0 {
		return nil
	}
	return sug
}

func candidateText(c collects.Candidate) string {
	t := c.Title
	if c.Year != "" {
		t += " (" + c.Year + ")"
	}
	t += " #" + c.From
	if c.To != "" {
		t += "–#" + c.To
	}
	return t
}

// collectsSuggestions computes (or returns the recent) list for the whole
// library. It scans every summary once; the memo keeps a filter box cheap.
func (s *Server) collectsSuggestions() ([]*CollectsSuggestion, error) {
	s.collectsSuggMu.Lock()
	defer s.collectsSuggMu.Unlock()
	if s.collectsSuggList != nil && time.Since(s.collectsSuggAt) < collectsSuggestTTL {
		return s.collectsSuggList, nil
	}
	books, err := s.backend.GetAllBooks()
	if err != nil {
		return nil, err
	}
	idx := s.volumeIndex()
	out := []*CollectsSuggestion{}
	for i := range books {
		if sug := suggestCollects(&books[i], idx, s.cvCache); sug != nil {
			out = append(out, sug)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		a, b := strings.ToLower(out[i].Series), strings.ToLower(out[j].Series)
		if a != b {
			return a < b
		}
		return out[i].Number < out[j].Number
	})
	s.collectsSuggList, s.collectsSuggAt = out, time.Now()
	return out, nil
}

func (s *Server) invalidateCollectsSuggestions() {
	s.collectsSuggMu.Lock()
	s.collectsSuggList = nil
	s.collectsSuggMu.Unlock()
}

// volumeIndex indexes the cached ComicVine volumes; nil without a cache.
func (s *Server) volumeIndex() *comicvine.VolumeIndex {
	if s.cvCache == nil {
		return nil
	}
	vols, err := s.cvCache.NamedVolumes()
	if err != nil {
		log.Warn().Err(err).Msg("Collects suggestions: could not read cached volumes")
		return nil
	}
	return comicvine.NewVolumeIndex(vols)
}

// collectsSuggestState is the memo; embedded in Server.
type collectsSuggestState struct {
	collectsSuggMu   sync.Mutex
	collectsSuggList []*CollectsSuggestion
	collectsSuggAt   time.Time
}

// handleCollectsSuggestions lists the proposals.
// GET /api/library/collects-suggestions?q=&min=high|medium|any&limit=&offset=
func (s *Server) handleCollectsSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	all, err := s.collectsSuggestions()
	if err != nil {
		http.Error(w, "Failed to compute suggestions", http.StatusInternalServerError)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	floor := 0
	switch r.URL.Query().Get("min") {
	case "high":
		floor = 2
	case "medium":
		floor = 1
	}
	var filtered []*CollectsSuggestion
	for _, sg := range all {
		if sg.best < floor {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(sg.Series+" "+sg.Title), q) {
			continue
		}
		filtered = append(filtered, sg)
	}
	limit, offset := parseLimitOffset(r, 25, 100)
	total := len(filtered)
	start := min(offset, total)
	end := min(start+limit, total)
	page := filtered[start:end]
	if page == nil {
		page = []*CollectsSuggestion{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"suggestions": page, "total": total, "limit": limit, "offset": offset, "has_more": end < total,
		"cache_available": s.cvCache != nil,
	})
}

// CollectsSuggestionAction is the body of POST /api/library/collects-suggestions/:bookId.
// Accept saves the proposed links (Keep lists the entry positions to save;
// empty means all). Dismiss stops proposing for this book.
type CollectsSuggestionAction struct {
	Action string `json:"action"`
	Keep   []int  `json:"keep"`
}

// handleCollectsSuggestionAction accepts or dismisses one book's proposal.
func (s *Server) handleCollectsSuggestionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/library/collects-suggestions/"), "/")
	var req CollectsSuggestionAction
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	backend := s.audited(audit.ActionCollects)
	book, err := backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	switch req.Action {
	case "dismiss":
		if collects.SetDismissed(book, true) {
			if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
				log.Error().Err(err).Str("book_id", id).Msg("Failed to dismiss collects suggestion")
				http.Error(w, "Failed to save book", http.StatusInternalServerError)
				return
			}
		}
	case "accept":
		if collects.Has(book) {
			http.Error(w, "this book already has links", http.StatusConflict)
			return
		}
		// Recompute from the book as it is now; never trust entries from the page.
		sug := suggestCollects(book, s.volumeIndex(), s.cvCache)
		if sug == nil {
			http.Error(w, "there is nothing to suggest for this book", http.StatusConflict)
			return
		}
		var picked []collects.Entry
		for i, e := range sug.Entries {
			if len(req.Keep) == 0 || containsInt(req.Keep, i) {
				picked = append(picked, e.Entry)
			}
		}
		if len(picked) == 0 {
			http.Error(w, "no entry chosen", http.StatusBadRequest)
			return
		}
		if err := collects.ValidateAll(picked); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if collects.Set(book, picked) {
			if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
				log.Error().Err(err).Str("book_id", id).Msg("Failed to save suggested collects links")
				http.Error(w, "Failed to save book", http.StatusInternalServerError)
				return
			}
		}
		s.invalidateVolumeSummaries()
	default:
		http.Error(w, "action must be accept or dismiss", http.StatusBadRequest)
		return
	}
	s.invalidateCollectsSuggestions()
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "action": req.Action})
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
