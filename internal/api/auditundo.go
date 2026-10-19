package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// Undo of field values from the audit log (tracker #14). Narrow on purpose:
// only fields whose old value the entry records in full can come back.
//
//   - Scalar fields the edit form may write (editableFields) and custom
//     values that are not comic_server_ bookkeeping are undoable.
//   - Slice/map fields (recorded as "N items"), file fields, dirty flags and
//     any comic_server_ custom value are refused with a reason.
//   - A field the user locked on the book (fieldlock.go) is never restored,
//     not even with force.
//   - The current value must still equal the entry's new value; otherwise the
//     field is a conflict and is only restored with force.
//   - Every restore is checked by reading the value back, so a field the
//     writer cannot reproduce exactly is refused rather than approximated.

// Item statuses.
const (
	undoOK          = "ok"          // current == entry's new value, can restore
	undoConflict    = "conflict"    // changed since; restored only with force
	undoUnchanged   = "unchanged"   // already equals the old value
	undoUnsupported = "unsupported" // cannot be undone, see Reason
	undoLocked      = "locked"      // the user locked this field on the book; never restored (tracker #27)
)

// UndoItem is one field of an entry in an undo preview/result.
type UndoItem struct {
	Field   string `json:"field"`
	Old     string `json:"old"`
	New     string `json:"new"`
	Current string `json:"current"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Applied bool   `json:"applied,omitempty"`
}

// UndoRequest is the body of POST /api/library/audit/undo.
type UndoRequest struct {
	EntryID int64 `json:"entry_id"`
	// Fields limits the undo to these entry fields; empty means every
	// undoable one.
	Fields []string `json:"fields"`
	// Apply false previews and writes nothing.
	Apply bool `json:"apply"`
	// Force also restores fields whose current value differs from the entry's.
	Force bool `json:"force"`
}

// UndoResponse is the preview or result.
type UndoResponse struct {
	EntryID int64      `json:"entry_id"`
	BookID  string     `json:"book_id"`
	Applied bool       `json:"applied"`
	Items   []UndoItem `json:"items"`
}

const customPrefix = "custom:"

// planUndo classifies each change of an entry against the book as it is now.
func planUndo(book *library.ComicBook, changes []audit.Change) []UndoItem {
	items := make([]UndoItem, 0, len(changes))
	locked := lockedFields(book)
	for _, c := range changes {
		it := UndoItem{Field: c.Field, Old: c.Old, New: c.New}
		if slices.Contains(locked, c.Field) {
			it.Status, it.Reason = undoLocked, "locked on this book"
			if key, ok := strings.CutPrefix(c.Field, customPrefix); ok {
				it.Current = library.CustomValuesMap(book.CustomValuesStore)[key]
			} else {
				it.Current, _ = audit.FieldValue(book, c.Field)
			}
			items = append(items, it)
			continue
		}
		if key, ok := strings.CutPrefix(c.Field, customPrefix); ok {
			it.Current = library.CustomValuesMap(book.CustomValuesStore)[key]
			if strings.HasPrefix(key, "comic_server_") {
				it.Status, it.Reason = undoUnsupported, "comic-server bookkeeping value"
			} else {
				classify(&it)
			}
			items = append(items, it)
			continue
		}
		cur, known := audit.FieldValue(book, c.Field)
		it.Current = cur
		if _, editable := editableFields[c.Field]; !known || !editable {
			it.Status, it.Reason = undoUnsupported, "not a field the edit form can write, or its old value is not recorded in full"
		} else {
			classify(&it)
		}
		items = append(items, it)
	}
	return items
}

func classify(it *UndoItem) {
	switch {
	case it.Current == it.Old:
		it.Status = undoUnchanged
	case it.Current == it.New:
		it.Status = undoOK
	default:
		it.Status = undoConflict
		it.Reason = "changed since this entry"
	}
}

// restoreField writes the entry's old value onto book and reports the
// ComicInfo/organize impact. It fails when the value read back is not the
// recorded old value.
func restoreField(book *library.ComicBook, it UndoItem) (comicInfo, organize bool, err error) {
	if key, ok := strings.CutPrefix(it.Field, customPrefix); ok {
		if it.Old == "" {
			book.CustomValuesStore = library.RemoveCustomValue(book.CustomValuesStore, key)
		} else {
			book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, key, it.Old)
		}
		if got := library.CustomValuesMap(book.CustomValuesStore)[key]; got != it.Old {
			return false, false, fmt.Errorf("%s: cannot be restored exactly", it.Field)
		}
		return false, false, nil
	}
	val := it.Old
	if it.Field == "Checked" { // the audit text is true/false, the writer takes Yes/No
		val = map[string]string{"true": "Yes", "false": "No"}[val]
	}
	if err := datamanager.SetFieldString(book, it.Field, val); err != nil {
		return false, false, fmt.Errorf("%s: %w", it.Field, err)
	}
	if got, _ := audit.FieldValue(book, it.Field); got != it.Old {
		return false, false, fmt.Errorf("%s: cannot be restored exactly", it.Field)
	}
	spec := editableFields[it.Field]
	return spec.comicInfo, spec.organize, nil
}

// handleAuditUndo previews or applies restoring the old field values of one
// audit entry. The undo is itself logged (action "undo").
// POST /api/library/audit/undo
func (s *Server) handleAuditUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil || s.configDB == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	var req UndoRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.EntryID <= 0 {
		http.Error(w, "entry_id is required", http.StatusBadRequest)
		return
	}
	row, found, err := s.configDB.GetAudit(req.EntryID)
	if err != nil {
		http.Error(w, "Failed to load audit entry", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "Audit entry not found (it may have been pruned)", http.StatusNotFound)
		return
	}
	var changes []audit.Change
	if err := json.Unmarshal([]byte(row.Changes), &changes); err != nil {
		http.Error(w, "Audit entry is unreadable", http.StatusInternalServerError)
		return
	}
	backend := s.audited(audit.ActionUndo)
	book, err := backend.GetBook(row.BookID)
	if err != nil || book == nil {
		http.Error(w, "Book no longer exists", http.StatusNotFound)
		return
	}
	if len(req.Fields) > 0 {
		kept := changes[:0:0]
		for _, c := range changes {
			if slices.Contains(req.Fields, c.Field) {
				kept = append(kept, c)
			}
		}
		changes = kept
	}
	items := planUndo(book, changes)
	resp := UndoResponse{EntryID: row.ID, BookID: row.BookID, Items: items}
	if !req.Apply {
		s.writeJSON(w, http.StatusOK, resp)
		return
	}

	// Compute everything on the copy first; one bad field writes nothing.
	comicInfo, organize, n := false, false, 0
	for i := range items {
		it := &items[i]
		if it.Status == undoUnsupported || it.Status == undoLocked || it.Status == undoUnchanged || (it.Status == undoConflict && !req.Force) {
			continue
		}
		ci, org, err := restoreField(book, *it)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		comicInfo, organize = comicInfo || ci, organize || org
		it.Applied = true
		n++
	}
	if n == 0 {
		http.Error(w, "Nothing to undo: every field is unchanged, unsupported, or changed since (use force to override)", http.StatusConflict)
		return
	}
	if comicInfo && book.FilePath != "" {
		book.ComicInfoIsDirty = true
	}
	if book.FilePath != "" {
		book.ComicBookIsDirty = true
	}
	if organize {
		workflow.RegressToStageIfPast(book, workflow.StageToMove)
	}
	if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
		log.Error().Err(err).Str("book_id", book.ID).Msg("Failed to save undo")
		http.Error(w, "Failed to save book", http.StatusInternalServerError)
		return
	}
	s.InvalidateWorkflowCache()
	resp.Applied = true
	s.writeJSON(w, http.StatusOK, resp)
}
