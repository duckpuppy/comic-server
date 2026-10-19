package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Manual lists (tracker #9, was comic-server-k1bt.4): ComicRack's plain
// "List", a ComicIdListItem holding an ordered set of book GUIDs. Created and
// renamed through the normal list endpoints (POST/PUT/DELETE
// /api/library/lists); membership is edited here:
//
//	POST /api/library/lists/:id/books         {book_ids | browse, index?}  add
//	POST /api/library/lists/:id/books/remove  {book_ids}                   remove
//	POST /api/library/lists/:id/books/move    {book_id, index}             reorder
//	GET  /api/library/manual-lists?book_id=                                pick a list
//
// Every write first drops members that no longer resolve to a book and
// duplicate members, as ComicRack's ComicIdListItem.OnGetBooks does, so a
// position here is the same position the list preview shows. CBL-imported
// reading lists (ComicReadingList) are not editable here: their membership
// belongs to the CBL file and a reimport would undo the edit.

// maxManualListAdd caps one add, like bulk edit.
const maxManualListAdd = maxBulkEditBooks

// isManualListType reports whether a list's membership is an editable id list.
func isManualListType(t string) bool {
	return strings.Contains(t, "IdListItem")
}

// ManualListAddRequest is the body of an add. BookIDs, when not empty, is
// the scope; otherwise Browse is (Browse's "select all N matching").
type ManualListAddRequest struct {
	BookIDs []string      `json:"book_ids,omitempty"`
	Browse  BrowseRequest `json:"browse"`
	// Index is where the new books go (0 = top). Omitted = the end.
	Index *int `json:"index,omitempty"`
}

// ManualListRemoveRequest is the body of a remove.
type ManualListRemoveRequest struct {
	BookIDs []string `json:"book_ids"`
}

// ManualListMoveRequest moves one member so it ends up at Index (0-based,
// clamped to the list).
type ManualListMoveRequest struct {
	BookID string `json:"book_id"`
	Index  int    `json:"index"`
}

// ManualListResult reports a membership change.
type ManualListResult struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"book_count"`
	Added     int    `json:"added,omitempty"`
	Already   int    `json:"already,omitempty"` // asked to add, already a member
	Unknown   int    `json:"unknown,omitempty"` // ids that are not books
	Removed   int    `json:"removed,omitempty"`
	Position  *int   `json:"position,omitempty"` // a moved book's new index
}

// ManualListSummary is one row of GET /api/library/manual-lists.
type ManualListSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"book_count"`
	// Contains is set only when the request named a book_id.
	Contains bool `json:"contains,omitempty"`
}

// insertMembers puts the ids of add not already in cur at index (clamped),
// keeping add's order. It returns the new slice and the ids it inserted.
func insertMembers(cur, add []string, index int) ([]string, []string) {
	have := make(map[string]bool, len(cur)+len(add))
	for _, id := range cur {
		have[id] = true
	}
	var fresh []string
	for _, id := range add {
		if id != "" && !have[id] {
			have[id] = true
			fresh = append(fresh, id)
		}
	}
	if len(fresh) == 0 {
		return cur, nil
	}
	index = max(0, min(index, len(cur)))
	out := make([]string, 0, len(cur)+len(fresh))
	out = append(out, cur[:index]...)
	out = append(out, fresh...)
	out = append(out, cur[index:]...)
	return out, fresh
}

// removeMembers drops every id in remove from cur and returns what it dropped.
func removeMembers(cur, remove []string) ([]string, []string) {
	drop := make(map[string]bool, len(remove))
	for _, id := range remove {
		drop[id] = true
	}
	out := make([]string, 0, len(cur))
	var removed []string
	for _, id := range cur {
		if drop[id] {
			removed = append(removed, id)
			continue
		}
		out = append(out, id)
	}
	return out, removed
}

// moveMember moves id so it sits at index (clamped). It returns the new
// slice and the final index, or ok=false when id is not a member.
func moveMember(cur []string, id string, index int) (out []string, at int, ok bool) {
	from := slices.Index(cur, id)
	if from < 0 {
		return cur, 0, false
	}
	out = slices.Delete(slices.Clone(cur), from, from+1)
	at = max(0, min(index, len(out)))
	return slices.Insert(out, at, id), at, true
}

// loadManualList finds an editable id list and returns a copy whose BookIds
// are its resolvable members in order (missing and duplicate ids dropped),
// plus those books by id. The copy is safe to change: the XML backend hands
// out a pointer into the live library.
func (s *Server) loadManualList(w http.ResponseWriter, listID string) (*library.ComicListItem, map[string]*library.ComicBook, bool) {
	found, err := s.backend.FindListByID(listID)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Error looking up list")
		http.Error(w, "Error looking up list", http.StatusInternalServerError)
		return nil, nil, false
	}
	if found == nil {
		http.Error(w, "List not found", http.StatusNotFound)
		return nil, nil, false
	}
	if !isManualListType(found.Type) {
		http.Error(w, "Only manual lists can be edited this way. Smart lists pick books by their rules, and a CBL reading list's books come from its file.", http.StatusConflict)
		return nil, nil, false
	}
	list := *found
	list.BookIds = slices.Clone(found.BookIds)
	books, err := s.backend.GetBooksForList(&list)
	if err != nil {
		log.Error().Err(err).Str("list_id", listID).Msg("Failed to read list members")
		http.Error(w, "Failed to read the list", http.StatusInternalServerError)
		return nil, nil, false
	}
	byID := make(map[string]*library.ComicBook, len(books))
	ids := make([]string, 0, len(books))
	for _, b := range books {
		if _, dup := byID[b.ID]; dup {
			continue
		}
		byID[b.ID] = b
		ids = append(ids, b.ID)
	}
	list.BookIds = ids
	return &list, byID, true
}

// saveManualList writes the list and stores its new counts. books must hold
// every member. Invalidate alone would keep serving the old count until a
// background refresh finished, so a page reloaded right after an edit
// showed the old number.
func (s *Server) saveManualList(w http.ResponseWriter, list *library.ComicListItem, books map[string]*library.ComicBook) bool {
	if err := s.backend.UpdateList(list); err != nil {
		log.Error().Err(err).Str("list_id", list.ID).Msg("Failed to save manual list")
		http.Error(w, "Failed to save the list", http.StatusInternalServerError)
		return false
	}
	unread := 0
	for _, id := range list.BookIds {
		if b := books[id]; b != nil && b.IsUnread() {
			unread++
		}
	}
	s.listCache.SetCounts(list.ID, len(list.BookIds), unread)
	return true
}

// auditListMembership records one entry per book added to (or removed from)
// a list, so the book's history shows it.
func (s *Server) auditListMembership(list *library.ComicListItem, books []*library.ComicBook, added bool) {
	if s.configDB == nil || len(books) == 0 {
		return
	}
	now := time.Now()
	rows := make([]configdb.AuditRow, 0, len(books))
	for _, b := range books {
		c := audit.Change{Field: "list", New: list.Name}
		if !added {
			c = audit.Change{Field: "list", Old: list.Name}
		}
		data, _ := json.Marshal([]audit.Change{c})
		rows = append(rows, configdb.AuditRow{
			At: now, BookID: b.ID, Action: audit.ActionReadingList, Series: b.Series, Number: b.Number,
			Changes: string(data),
		})
	}
	if err := s.configDB.AppendAudit(rows); err != nil {
		log.Warn().Err(err).Str("list_id", list.ID).Msg("Failed to write audit log")
	}
}

// writableLibrary answers 503/403 when list membership cannot be saved.
func (s *Server) writableLibrary(w http.ResponseWriter) bool {
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return false
	}
	if !s.backend.CanPersist() {
		http.Error(w, "Library is read-only", http.StatusForbidden)
		return false
	}
	return true
}

// routeManualListBooks handles /api/library/lists/:id/books[/remove|/move].
// It reports false when sub is not one of its paths.
func (s *Server) routeManualListBooks(w http.ResponseWriter, r *http.Request, listID, sub string) bool {
	var handle func(http.ResponseWriter, *http.Request, string)
	switch sub {
	case "":
		handle = s.handleManualListAdd
	case "/remove":
		handle = s.handleManualListRemove
	case "/move":
		handle = s.handleManualListMove
	default:
		return false
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	if !s.writableLibrary(w) {
		return true
	}
	s.manualListMu.Lock()
	defer s.manualListMu.Unlock()
	handle(w, r, listID)
	return true
}

func (s *Server) handleManualListAdd(w http.ResponseWriter, r *http.Request, listID string) {
	var req ManualListAddRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	list, members, ok := s.loadManualList(w, listID)
	if !ok {
		return
	}
	ids, err := s.bulkEditScope(BulkEditRequest{BookIDs: req.BookIDs, Browse: req.Browse})
	if err != nil {
		log.Error().Err(err).Msg("Failed to resolve the books to add")
		http.Error(w, "Failed to resolve the selection", http.StatusInternalServerError)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "No books selected", http.StatusBadRequest)
		return
	}
	if len(ids) > maxManualListAdd {
		http.Error(w, fmt.Sprintf("%d books selected; the limit is %d. Narrow the filter.", len(ids), maxManualListAdd), http.StatusRequestEntityTooLarge)
		return
	}

	res := ManualListResult{ID: list.ID, Name: list.Name}
	books := make(map[string]*library.ComicBook, len(ids))
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		b, err := s.backend.GetBook(id)
		if err != nil {
			log.Error().Err(err).Str("book_id", id).Msg("Failed to look up book")
			http.Error(w, "Failed to look up a book", http.StatusInternalServerError)
			return
		}
		if b == nil {
			res.Unknown++
			continue
		}
		books[id] = b
		members[id] = b
		valid = append(valid, id)
	}
	index := len(list.BookIds)
	if req.Index != nil {
		index = *req.Index
	}
	var fresh []string
	list.BookIds, fresh = insertMembers(list.BookIds, valid, index)
	res.Added = len(fresh)
	res.Already = len(valid) - len(fresh)
	res.BookCount = len(list.BookIds)
	if len(fresh) > 0 {
		if !s.saveManualList(w, list, members) {
			return
		}
		added := make([]*library.ComicBook, len(fresh))
		for i, id := range fresh {
			added[i] = books[id]
		}
		s.auditListMembership(list, added, true)
		log.Info().Str("list_id", list.ID).Str("name", list.Name).Int("added", len(fresh)).Msg("Books added to manual list")
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleManualListRemove(w http.ResponseWriter, r *http.Request, listID string) {
	var req ManualListRemoveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.BookIDs) == 0 {
		http.Error(w, "book_ids is required", http.StatusBadRequest)
		return
	}
	list, books, ok := s.loadManualList(w, listID)
	if !ok {
		return
	}
	var removed []string
	list.BookIds, removed = removeMembers(list.BookIds, req.BookIDs)
	if len(removed) > 0 && !s.saveManualList(w, list, books) {
		return
	}
	gone := make([]*library.ComicBook, len(removed))
	for i, id := range removed {
		gone[i] = books[id]
	}
	s.auditListMembership(list, gone, false)
	if len(removed) > 0 {
		log.Info().Str("list_id", list.ID).Str("name", list.Name).Int("removed", len(removed)).Msg("Books removed from manual list")
	}
	s.writeJSON(w, http.StatusOK, ManualListResult{ID: list.ID, Name: list.Name, BookCount: len(list.BookIds), Removed: len(removed)})
}

func (s *Server) handleManualListMove(w http.ResponseWriter, r *http.Request, listID string) {
	var req ManualListMoveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.BookID == "" {
		http.Error(w, "book_id is required", http.StatusBadRequest)
		return
	}
	list, books, ok := s.loadManualList(w, listID)
	if !ok {
		return
	}
	var at int
	list.BookIds, at, ok = moveMember(list.BookIds, req.BookID, req.Index)
	if !ok {
		http.Error(w, "That book is not in this list", http.StatusNotFound)
		return
	}
	if !s.saveManualList(w, list, books) {
		return
	}
	log.Info().Str("list_id", list.ID).Str("book_id", req.BookID).Int("index", at).Msg("Manual list reordered")
	s.writeJSON(w, http.StatusOK, ManualListResult{ID: list.ID, Name: list.Name, BookCount: len(list.BookIds), Position: &at})
}

// handleManualLists GET /api/library/manual-lists?book_id= lists every manual
// list (folders flattened), sorted by name, for the "Add to list" picker.
// With book_id each row says whether that book is already a member.
func (s *Server) handleManualLists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	all, err := s.backend.GetAllLists()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get lists from backend")
		http.Error(w, "Failed to get lists", http.StatusInternalServerError)
		return
	}
	bookID := r.URL.Query().Get("book_id")
	out := make([]ManualListSummary, 0)
	var walk func(items []library.ComicListItem)
	walk = func(items []library.ComicListItem) {
		for i := range items {
			l := &items[i]
			if isManualListType(l.Type) {
				count, _ := s.getListCounts(l)
				out = append(out, ManualListSummary{
					ID: l.ID, Name: l.Name, BookCount: count,
					Contains: bookID != "" && slices.Contains(l.BookIds, bookID),
				})
			}
			walk(l.ChildItems)
		}
	}
	walk(all)
	slices.SortStableFunc(out, func(a, b ManualListSummary) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	s.writeJSON(w, http.StatusOK, map[string]any{"lists": out})
}
