package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/datamanager"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// Book edit form (ComicRack's Info dialog for one book).
//
// POST /api/library/books/:id/edit
//
// Only the fields sent are written, so the form sends just what the user
// changed and a concurrent change to another field survives. Values go
// through datamanager.SetFieldString, the same typed writer the Data
// Manager uses.

// editableFields are the fields the form may write. comicInfo marks the
// ones that go into ComicInfo.xml (cbzconvert.buildComicInfoXML), so
// changing one marks the file out of date; organize marks the ones that
// can change where the Library Organizer files the book.
var editableFields = map[string]struct{ comicInfo, organize bool }{
	"Series": {true, true}, "Number": {true, true}, "Count": {true, true}, "Volume": {true, true},
	"Title": {true, true}, "AlternateSeries": {true, true}, "AlternateNumber": {true, true},
	"AlternateCount": {true, true}, "StoryArc": {true, true}, "SeriesGroup": {true, true},
	"Year": {true, true}, "Month": {true, true}, "Day": {true, true},
	"Publisher": {true, true}, "Imprint": {true, true}, "Format": {true, true},
	"LanguageISO": {true, true}, "Genre": {true, true}, "Web": {true, true}, "AgeRating": {true, true},
	"Writer": {true, true}, "Penciller": {true, true}, "Inker": {true, true}, "Colorist": {true, true},
	"Letterer": {true, true}, "CoverArtist": {true, true}, "Editor": {true, true}, "Translator": {true, true},
	"Characters": {true, true}, "Teams": {true, true}, "MainCharacterOrTeam": {true, true},
	"Locations": {true, true}, "Tags": {true, true}, "ScanInformation": {true, true},
	"BlackAndWhite": {true, true}, "Manga": {true, true},
	"Summary": {true, false}, "Notes": {true, false}, "Review": {true, false},
	"Rating": {true, false}, "CommunityRating": {true, false},
	"Checked": {false, false}, "SeriesComplete": {false, false}, "ISBN": {false, false},
	"BookPrice": {false, false}, "BookAge": {false, false}, "BookCondition": {false, false},
	"BookStore": {false, false}, "BookOwner": {false, false}, "BookCollectionStatus": {false, false},
	"BookNotes": {false, false}, "BookLocation": {false, false},
}

// multilineFields keep their inner line breaks; every other field is one line.
var multilineFields = map[string]bool{"Summary": true, "Notes": true, "Review": true, "BookNotes": true}

// BookEditRequest is the body of POST /api/library/books/:id/edit.
type BookEditRequest struct {
	// Fields maps a field name to its new value: a string, number or bool.
	// An empty string clears the field.
	Fields map[string]any `json:"fields"`
	// ListAdd and ListRemove change a list field (Genre, Writer, Tags...)
	// by adding or removing items instead of replacing the whole value
	// (bulk edit, comic-server-k1bt.2). Each value is a comma-separated
	// list of items. A field may not also appear in Fields.
	ListAdd    map[string]string `json:"list_add,omitempty"`
	ListRemove map[string]string `json:"list_remove,omitempty"`
	// Read is "read", "unread" or "" (leave the read state alone).
	Read string `json:"read"`
}

// listFields hold comma-separated items, so they can take ListAdd/ListRemove.
var listFields = map[string]bool{
	"Genre": true, "Writer": true, "Penciller": true, "Inker": true, "Colorist": true,
	"Letterer": true, "CoverArtist": true, "Editor": true, "Translator": true,
	"Characters": true, "Teams": true, "Locations": true, "Tags": true,
}

// splitItems splits a comma-separated list into trimmed, non-empty items.
func splitItems(list string) []string {
	var items []string
	for _, v := range strings.Split(list, ",") {
		if v = strings.TrimSpace(v); v != "" {
			items = append(items, v)
		}
	}
	return items
}

// mergeListItems adds then removes items, comparing without regard to case.
// Existing items keep their spelling and order; added ones go on the end.
func mergeListItems(current string, add, remove []string) string {
	items := splitItems(current)
	has := func(list []string, item string) bool {
		return slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, item) })
	}
	for _, a := range add {
		if !has(items, a) {
			items = append(items, a)
		}
	}
	kept := items[:0:0]
	for _, it := range items {
		if !has(remove, it) {
			kept = append(kept, it)
		}
	}
	return strings.Join(kept, ", ")
}

// expandListOps turns ListAdd/ListRemove into whole-field values, using the
// book's current value, so applyBookEdit treats them like any other field.
func expandListOps(book *library.ComicBook, req BookEditRequest) (map[string]any, error) {
	fields := make(map[string]any, len(req.Fields)+len(req.ListAdd)+len(req.ListRemove))
	for k, v := range req.Fields {
		fields[k] = v
	}
	names := map[string]bool{}
	for f := range req.ListAdd {
		names[f] = true
	}
	for f := range req.ListRemove {
		names[f] = true
	}
	for f := range names {
		if !listFields[f] {
			return nil, fmt.Errorf("%s is not a list field", f)
		}
		if _, dup := req.Fields[f]; dup {
			return nil, fmt.Errorf("%s: cannot replace and add/remove in one edit", f)
		}
		cur, _ := datamanager.GetFieldString(book, f)
		fields[f] = mergeListItems(cur, splitItems(req.ListAdd[f]), splitItems(req.ListRemove[f]))
	}
	return fields, nil
}

// BookEditResponse lists the fields that actually changed.
type BookEditResponse struct {
	BookID  string   `json:"book_id"`
	Changed []string `json:"changed"`
}

// editValueString turns a JSON value into the string SetFieldString takes.
func editValueString(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		if t {
			return "Yes", nil
		}
		return "No", nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(t), nil
	default:
		return "", fmt.Errorf("unsupported value type %T", v)
	}
}

// validateEditValue checks ranges and enum values the typed writer accepts
// without complaint. value is already trimmed.
func validateEditValue(field, value string) error {
	num := func(lo, hi float64) error {
		if value == "" {
			return nil
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("%s must be a number", field)
		}
		if f < lo || f > hi {
			return fmt.Errorf("%s must be between %g and %g", field, lo, hi)
		}
		return nil
	}
	oneOf := func(allowed ...string) error {
		if value == "" || slices.Contains(allowed, value) {
			return nil
		}
		return fmt.Errorf("%s must be one of: %s", field, strings.Join(allowed, ", "))
	}
	switch field {
	case "Rating", "CommunityRating":
		return num(0, 5)
	case "Month":
		return num(0, 12)
	case "Day":
		return num(0, 31)
	case "Year":
		return num(0, 9999)
	case "Count", "Volume", "AlternateCount":
		return num(0, 1e9)
	case "BookPrice":
		return num(0, 1e9)
	case "BlackAndWhite", "SeriesComplete":
		return oneOf("Unknown", "Yes", "No")
	case "Manga":
		return oneOf("Unknown", "Yes", "No", "YesAndRightToLeft")
	case "Checked":
		return oneOf("Yes", "No")
	}
	return nil
}

// applyBookEdit writes req onto book and returns the changed field names,
// sorted. "Read" is reported as a change when the read state moved.
func applyBookEdit(book *library.ComicBook, req BookEditRequest, now time.Time) ([]string, error) {
	var changed []string
	comicInfoChanged, organizeChanged := false, false
	fields, err := expandListOps(book, req)
	if err != nil {
		return nil, err
	}
	for field, raw := range fields {
		spec, ok := editableFields[field]
		if !ok {
			return nil, fmt.Errorf("field %q cannot be edited", field)
		}
		value, err := editValueString(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		if multilineFields[field] {
			value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
		} else {
			value = strings.Join(strings.Fields(value), " ")
		}
		if field == "BlackAndWhite" || field == "Manga" || field == "SeriesComplete" {
			if value == "Unknown" {
				value = "" // the library omits Unknown
			}
		}
		if err := validateEditValue(field, value); err != nil {
			return nil, err
		}
		before, _ := datamanager.GetFieldString(book, field)
		if err := datamanager.SetFieldString(book, field, value); err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		if after, _ := datamanager.GetFieldString(book, field); after == before {
			continue
		}
		changed = append(changed, field)
		comicInfoChanged = comicInfoChanged || spec.comicInfo
		organizeChanged = organizeChanged || spec.organize
	}

	switch req.Read {
	case "":
	case "read":
		if book.IsUnread() {
			markRead(book, now)
			changed = append(changed, "Read")
		}
	case "unread":
		if book.OpenCount > 0 || book.CurrentPage > 0 || book.LastPageRead > 0 {
			markUnread(book)
			changed = append(changed, "Read")
		}
	default:
		return nil, fmt.Errorf(`read must be "read", "unread" or empty`)
	}

	if len(changed) == 0 {
		return changed, nil
	}
	// ComicRack sets the same dirty flags when its Info dialog changes a
	// book: ComicInfo.xml for its own fields, ComicBook.xml for any field.
	if comicInfoChanged && book.FilePath != "" {
		book.ComicInfoIsDirty = true
	}
	if book.FilePath != "" {
		book.ComicBookIsDirty = true
	}
	// Like a Data Manager run: a book already filed may now belong
	// somewhere else, so send it back to the Organize step.
	if organizeChanged {
		workflow.RegressToStageIfPast(book, workflow.StageToMove)
	}
	slices.Sort(changed)
	return changed, nil
}

// markRead and markUnread follow ComicBook.MarkAsRead / MarkAsNotRead in
// ComicRackCE.
func markRead(b *library.ComicBook, now time.Time) {
	if b.OpenCount == 0 {
		b.OpenCount = 1
	}
	b.OpenedTime = library.ComicTime{Time: now}
	if b.PageCount > 0 {
		b.CurrentPage = b.PageCount - 1
		b.LastPageRead = b.PageCount - 1
	}
}

func markUnread(b *library.ComicBook) {
	b.OpenedTime = library.ComicTime{}
	b.CurrentPage = 0
	b.LastPageRead = 0
	b.OpenCount = 0
}

// handleBookEdit POST /api/library/books/:id/edit
func (s *Server) handleBookEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.backend == nil {
		http.Error(w, "Library not available", http.StatusServiceUnavailable)
		return
	}
	id := bookDetailID(r.URL.Path, "/edit")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var req BookEditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	backend := s.audited(audit.ActionEdit)
	book, err := backend.GetBook(id)
	if err != nil || book == nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	changed, err := applyBookEdit(book, req, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(changed) > 0 {
		if err := backend.UpdateBooks([]*library.ComicBook{book}); err != nil {
			log.Error().Err(err).Str("book_id", id).Msg("Failed to save book edit")
			http.Error(w, "Failed to save book", http.StatusInternalServerError)
			return
		}
		s.InvalidateWorkflowCache()
	}
	s.writeJSON(w, http.StatusOK, BookEditResponse{BookID: id, Changed: append([]string{}, changed...)})
}
