package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// DMFieldChange is the wire shape of one datamanager.FieldChange.
type DMFieldChange struct {
	Field  string `json:"field"`
	Custom bool   `json:"custom"`
	Old    string `json:"old"`
	New    string `json:"new"`
}

// DMBookChange is every field a full Data Manager rule run would change on
// one book - a book with no changes is omitted from the response entirely,
// so the preview only ever lists books actually affected.
type DMBookChange struct {
	BookID  string          `json:"book_id"`
	Series  string          `json:"series"`
	Number  string          `json:"number"`
	Title   string          `json:"title"`
	Changes []DMFieldChange `json:"changes"`
}

// DMRunResult is the response for both preview and apply. Applied is false
// for a preview (nothing was written) and true once a run has actually
// been committed via the backend. For apply, Changed/Books describe what
// was ACTUALLY COMMITTED (the selected subset, or everything if no
// selection was given), not the full set of books that matched - see
// selectForApply.
type DMRunResult struct {
	Processed int            `json:"processed"`
	Changed   int            `json:"changed"`
	Applied   bool           `json:"applied"`
	Books     []DMBookChange `json:"books"`
	Errors    []string       `json:"errors,omitempty"`

	// Limit/Offset/HasMore are set only by the whole-library preview
	// (comic-server-dpq), which pages through Books to keep the response
	// bounded for a ~66K-book library - Changed always reports the TOTAL
	// count of books that would change, even when Books is just one page
	// of that total. Zero-valued for the list-scoped endpoints, which
	// don't paginate (a single smart list's match set is already bounded
	// by the list itself).
	Limit   int  `json:"limit,omitempty"`
	Offset  int  `json:"offset,omitempty"`
	HasMore bool `json:"has_more,omitempty"`
}

// DMFieldSelector identifies one field change within one book, for
// field-level selective apply (comic-server-z93) - the finer-grained
// sibling of book-level selection (comic-server-dpq): "keep the
// SeriesGroup change but skip the Tags change on the same book".
type DMFieldSelector struct {
	BookID string `json:"book_id"`
	Field  string `json:"field"`
	Custom bool   `json:"custom"`
}

// DMApplyRequest is the optional JSON body for an apply request -
// selective apply (comic-server-dpq, comic-server-z93). Fields takes
// precedence when non-empty: only those exact book+field pairs are
// committed, and BookIDs is ignored. Otherwise BookIDs selects whole
// books (every one of that book's changes). Both empty means "apply
// everything that changed", matching the original whole-run-at-once
// behavior.
type DMApplyRequest struct {
	BookIDs []string          `json:"book_ids,omitempty"`
	Fields  []DMFieldSelector `json:"fields,omitempty"`
}

// handleDataManagerPreview runs every enabled Data Manager ruleset against
// every book currently matched by one smart list (comic-server-764's
// design decision - not whole-library, not ad-hoc filters, see
// comic-server-joj for that) WITHOUT writing anything, so the UI can show
// an accurate before/after diff before the user commits. Never mutates a
// book pointer shared with the backend's cached library snapshot - every
// run works on its own copy.
// POST /api/library/lists/:listId/datamanager-preview
func (s *Server) handleDataManagerPreview(w http.ResponseWriter, r *http.Request) {
	s.runDataManagerList(w, r, "/datamanager-preview", false)
}

// handleDataManagerApply does the same full rule run as
// handleDataManagerPreview, then commits the resulting changes via
// Backend.UpdateBooks - every changed book by default, or only the
// caller's selected book_ids (comic-server-dpq's selective apply).
// POST /api/library/lists/:listId/datamanager-apply
func (s *Server) handleDataManagerApply(w http.ResponseWriter, r *http.Request) {
	s.runDataManagerList(w, r, "/datamanager-apply", true)
}

func (s *Server) runDataManagerList(w http.ResponseWriter, r *http.Request, suffix string, apply bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Configuration database not available", http.StatusServiceUnavailable)
		return
	}

	listID := listIDFromSubPath(r.URL.Path, suffix)
	list, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up smart list for Data Manager run")
		http.Error(w, "Error looking up smart list", http.StatusInternalServerError)
		return
	}
	if list == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return
	}

	rulesets, err := datamanager.LoadRulesets(s.configDB)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load Data Manager rules")
		http.Error(w, "Failed to load Data Manager rules", http.StatusInternalServerError)
		return
	}
	if len(rulesets) == 0 {
		http.Error(w, "No Data Manager rules configured - import a dataman.dat file first", http.StatusUnprocessableEntity)
		return
	}

	books, err := s.backend.MatchBooks(list)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to match books: %v", err), http.StatusInternalServerError)
		return
	}

	var bookIDs []string
	var fields []DMFieldSelector
	if apply {
		req, err := parseDMApplyRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		bookIDs = req.BookIDs
		fields = req.Fields
	}

	result := s.runDataManagerOverBooks(books, rulesets, apply, bookIDs, fields)
	s.writeJSON(w, http.StatusOK, result)
}

// handleDataManagerPreviewLibrary is handleDataManagerPreview's
// whole-library counterpart (comic-server-dpq's "Apply All"): the real
// dataman.dat's rules (series-grouping, tagging) are meant to apply
// globally, not just to whatever smart list happens to already exist, so
// this scans every book in the library rather than one list's match set.
// Paginated (limit/offset, same convention as the list preview endpoint)
// since a full diff table for a ~66K-book library would be far too much
// to load in the browser at once - Changed always reports the total
// count, Books is just the requested page of it.
// POST /api/library/datamanager-preview
func (s *Server) handleDataManagerPreviewLibrary(w http.ResponseWriter, r *http.Request) {
	s.runDataManagerLibrary(w, r, false)
}

// handleDataManagerApplyLibrary is handleDataManagerApply's whole-library
// counterpart. Unlike the paginated preview, apply is not paginated: with
// no selection given it commits every changed book in the library in one
// action (the literal "Apply All"), and with book_ids or fields given it
// commits only that (typically much smaller, user-selected) set - see
// selectForApply and selectForApplyFields.
// POST /api/library/datamanager-apply
func (s *Server) handleDataManagerApplyLibrary(w http.ResponseWriter, r *http.Request) {
	s.runDataManagerLibrary(w, r, true)
}

func (s *Server) runDataManagerLibrary(w http.ResponseWriter, r *http.Request, apply bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	if s.configDB == nil {
		http.Error(w, "Configuration database not available", http.StatusServiceUnavailable)
		return
	}

	rulesets, err := datamanager.LoadRulesets(s.configDB)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load Data Manager rules")
		http.Error(w, "Failed to load Data Manager rules", http.StatusInternalServerError)
		return
	}
	if len(rulesets) == 0 {
		http.Error(w, "No Data Manager rules configured - import a dataman.dat file first", http.StatusUnprocessableEntity)
		return
	}

	allBooks, err := s.backend.GetAllBooks()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load library: %v", err), http.StatusInternalServerError)
		return
	}
	books := make([]*library.ComicBook, len(allBooks))
	for i := range allBooks {
		books[i] = &allBooks[i]
	}

	var bookIDs []string
	var fields []DMFieldSelector
	if apply {
		req, err := parseDMApplyRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		bookIDs = req.BookIDs
		fields = req.Fields
	}

	result := s.runDataManagerOverBooks(books, rulesets, apply, bookIDs, fields)

	if !apply {
		result.Limit, result.Offset = parseLimitOffset(r, 20, 100)
		total := len(result.Books)
		start := min(result.Offset, total)
		end := min(start+result.Limit, total)
		result.HasMore = end < total
		result.Books = result.Books[start:end]
	}

	s.writeJSON(w, http.StatusOK, result)
}

// runDataManagerOverBooks evaluates rulesets against candidates and,
// if apply is true, commits the result via Backend.UpdateBooks. Selection
// precedence matches DMApplyRequest: non-empty fieldSelectors commits only
// those exact book+field pairs (comic-server-z93); otherwise non-empty
// bookIDs commits whole books (comic-server-dpq); otherwise every changed
// book is committed. Every path re-verifies the selection against this
// fresh run rather than trusting a possibly-stale client-held preview.
func (s *Server) runDataManagerOverBooks(candidates []*library.ComicBook, rulesets []datamanager.Ruleset, apply bool, bookIDs []string, fieldSelectors []DMFieldSelector) DMRunResult {
	changes, errs, updated := dmEvaluate(candidates, rulesets)
	result := DMRunResult{Processed: len(candidates), Applied: apply, Errors: errs}

	if !apply {
		result.Changed = len(changes)
		result.Books = changes
		return result
	}

	var selectedChanges []DMBookChange
	var toUpdate []*library.ComicBook
	if len(fieldSelectors) > 0 {
		originals := make(map[string]*library.ComicBook, len(candidates))
		for _, b := range candidates {
			originals[b.ID] = b
		}
		selectedChanges, toUpdate = selectForApplyFields(changes, originals, fieldSelectors)
	} else {
		selectedChanges, toUpdate = selectForApply(changes, updated, bookIDs)
	}
	result.Changed = len(toUpdate)
	result.Books = selectedChanges

	if len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save Data Manager rule run")
			result.Errors = append(result.Errors, err.Error())
		}
	}
	return result
}

// dmEvaluate runs rulesets against every book in candidates, working on a
// copy of each (never the pointer the caller passed in, which may be
// shared with the backend's cached library snapshot - see
// SQLiteBackend.cachedLibrary's own doc comment on why nothing may mutate
// a cached book in place). Returns every book with at least one change,
// plus the working copy for each (keyed by book ID) so a caller can decide
// which subset to actually persist without re-running the rules.
func dmEvaluate(candidates []*library.ComicBook, rulesets []datamanager.Ruleset) (changes []DMBookChange, errs []string, updated map[string]*library.ComicBook) {
	updated = make(map[string]*library.ComicBook)
	for _, book := range candidates {
		working := *book
		cs, err := datamanager.ApplyAll(&working, rulesets)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", book.ID, err))
		}
		if len(cs) == 0 {
			continue
		}

		wireChanges := make([]DMFieldChange, len(cs))
		for i, c := range cs {
			wireChanges[i] = DMFieldChange{Field: c.Field, Custom: c.Custom, Old: c.Old, New: c.New}
		}
		changes = append(changes, DMBookChange{
			BookID:  book.ID,
			Series:  book.Series,
			Number:  book.Number,
			Title:   book.Title,
			Changes: wireChanges,
		})
		updated[book.ID] = &working
	}
	return changes, errs, updated
}

// selectForApply picks which of changes/updated to actually persist.
// bookIDs empty means "apply all" (the original whole-run-at-once
// behavior); otherwise only the requested ids are committed, and any
// requested id no longer present in updated (the book stopped matching or
// stopped needing a change between preview and apply) is silently
// skipped rather than erroring - selection is re-verified against this
// run's own fresh results, not the client's possibly-stale preview.
func selectForApply(changes []DMBookChange, updated map[string]*library.ComicBook, bookIDs []string) (selected []DMBookChange, toUpdate []*library.ComicBook) {
	if len(bookIDs) == 0 {
		for _, c := range changes {
			selected = append(selected, c)
			toUpdate = append(toUpdate, updated[c.BookID])
		}
		return selected, toUpdate
	}

	want := make(map[string]bool, len(bookIDs))
	for _, id := range bookIDs {
		want[id] = true
	}
	for _, c := range changes {
		if !want[c.BookID] {
			continue
		}
		selected = append(selected, c)
		toUpdate = append(toUpdate, updated[c.BookID])
	}
	return selected, toUpdate
}

// selectForApplyFields builds a partial commit set for field-level
// selective apply (comic-server-z93): for each requested (book_id, field,
// custom) selector, if that exact field change is still present in this
// run's fresh changes, apply ONLY that field's new value onto a copy of
// the book's ORIGINAL (pre-rule-run) state - not the working copy every
// changed field was written into - so any of that book's OTHER changed
// fields are left untouched, even ones that also matched. A selector that
// no longer matches a fresh change (the book stopped matching, or that
// specific field stopped needing a change) is silently skipped, same
// re-verification policy as selectForApply's book-level selection.
func selectForApplyFields(changes []DMBookChange, originals map[string]*library.ComicBook, selectors []DMFieldSelector) (selected []DMBookChange, toUpdate []*library.ComicBook) {
	byBook := make(map[string]DMBookChange, len(changes))
	for _, c := range changes {
		byBook[c.BookID] = c
	}

	wantByBook := make(map[string][]DMFieldSelector)
	var order []string
	for _, sel := range selectors {
		if _, seen := wantByBook[sel.BookID]; !seen {
			order = append(order, sel.BookID)
		}
		wantByBook[sel.BookID] = append(wantByBook[sel.BookID], sel)
	}

	for _, bookID := range order {
		bookChange, ok := byBook[bookID]
		if !ok {
			continue
		}
		original, ok := originals[bookID]
		if !ok {
			continue
		}

		working := *original
		var appliedChanges []DMFieldChange
		for _, sel := range wantByBook[bookID] {
			var match *DMFieldChange
			for i := range bookChange.Changes {
				if bookChange.Changes[i].Field == sel.Field && bookChange.Changes[i].Custom == sel.Custom {
					match = &bookChange.Changes[i]
					break
				}
			}
			if match == nil {
				continue
			}
			if err := applyFieldValue(&working, match.Field, match.Custom, match.New); err != nil {
				// Already validated once during the rules run that produced
				// this exact field change, so a write error here would mean
				// something changed underneath us - skip defensively rather
				// than fail the whole request over one field.
				continue
			}
			appliedChanges = append(appliedChanges, *match)
		}
		if len(appliedChanges) == 0 {
			continue
		}

		selected = append(selected, DMBookChange{
			BookID:  bookChange.BookID,
			Series:  bookChange.Series,
			Number:  bookChange.Number,
			Title:   bookChange.Title,
			Changes: appliedChanges,
		})
		toUpdate = append(toUpdate, &working)
	}
	return selected, toUpdate
}

// applyFieldValue writes value to field on book as either a built-in field
// (via datamanager.SetFieldString) or a custom value (via
// library.SetCustomValue), matching how datamanager.ApplyAll itself
// distinguishes the two - see internal/datamanager/actions.go's
// writeAnyField for the same split.
func applyFieldValue(book *library.ComicBook, field string, custom bool, value string) error {
	if custom {
		book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, field, value)
		return nil
	}
	return datamanager.SetFieldString(book, field, value)
}

// parseDMApplyRequest reads an optional JSON body for an apply request -
// a missing or empty body is not an error (means "apply everything"),
// matching apply's pre-selective-apply behavior of taking no body at all.
func parseDMApplyRequest(r *http.Request) (DMApplyRequest, error) {
	var req DMApplyRequest
	if r.Body == nil {
		return req, nil
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return req, fmt.Errorf("failed to read request body: %w", err)
	}
	if len(data) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return req, fmt.Errorf("invalid request body: %w", err)
	}
	return req, nil
}

// parseLimitOffset reads ?limit=&offset= query params with the same
// clamping convention handleGetListPreview already uses.
func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	query := r.URL.Query()
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

func listIDFromSubPath(path, suffix string) string {
	s := strings.TrimPrefix(path, "/api/library/lists/")
	return strings.TrimSuffix(s, suffix)
}
