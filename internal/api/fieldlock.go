package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
)

// Field locks (tracker #27). A user can lock a field on one book; an undo of
// any audit entry then leaves that field alone, with or without Force. The
// lock stops undo only. Scrapes, Data Manager rules and bulk edits ignore it.
//
// Stored on the book as the comic_server_locked_fields custom value, the
// field names joined with "+", the way accepted flags are. The names are the
// audit field names: an editable field ("Series") or "custom:<key>".

const lockedFieldsKey = "comic_server_locked_fields"

// lockedFields returns the book's locked field names.
func lockedFields(book *library.ComicBook) []string {
	v, _ := library.GetCustomValue(book.CustomValuesStore, lockedFieldsKey)
	if v == "" {
		return []string{}
	}
	return strings.Split(v, "+")
}

// setFieldLocked locks or unlocks one field and reports whether anything changed.
func setFieldLocked(book *library.ComicBook, field string, locked bool) bool {
	cur := lockedFields(book)
	has := slices.Contains(cur, field)
	if has == locked {
		return false
	}
	if locked {
		cur = append(cur, field)
		slices.Sort(cur)
	} else {
		cur = slices.DeleteFunc(cur, func(f string) bool { return f == field })
	}
	if len(cur) == 0 {
		book.CustomValuesStore = library.RemoveCustomValue(book.CustomValuesStore, lockedFieldsKey)
	} else {
		book.CustomValuesStore = library.SetCustomValue(book.CustomValuesStore, lockedFieldsKey, strings.Join(cur, "+"))
	}
	return true
}

// lockableField reports whether a field name may be locked: a field the edit
// form can write, or a custom value that is not comic-server bookkeeping.
func lockableField(field string) bool {
	if strings.Contains(field, "+") {
		return false
	}
	if key, ok := strings.CutPrefix(field, customPrefix); ok {
		return key != "" && !strings.HasPrefix(key, "comic_server_")
	}
	_, ok := editableFields[field]
	return ok
}

// LockFieldRequest is the body of POST /api/library/books/:id/lock-field.
type LockFieldRequest struct {
	Field  string `json:"field"`
	Locked bool   `json:"locked"` // false unlocks
}

// handleLockField locks or unlocks one field of one book against undo.
// POST /api/library/books/:id/lock-field
func (s *Server) handleLockField(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	id := bookDetailID(r.URL.Path, "/lock-field")
	var req LockFieldRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	// Unlocking is always allowed, so a lock on a name that is no longer
	// lockable can still be removed.
	if req.Locked && !lockableField(req.Field) {
		http.Error(w, "This field cannot be locked", http.StatusBadRequest)
		return
	}
	backend := s.audited(audit.ActionEdit)
	book, err := backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	if setFieldLocked(book, req.Field, req.Locked) {
		if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
			log.Error().Err(err).Str("book_id", id).Msg("Failed to save field lock")
			http.Error(w, "Failed to save book", http.StatusInternalServerError)
			return
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"book_id": id, "locked_fields": lockedFields(book)})
}
