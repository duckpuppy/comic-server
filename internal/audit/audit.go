// Package audit records what each pipeline action changed on each book
// (comic-server-vqsj): an append-only, field-level before/after history that
// powers a per-comic "history" view and stage rollback.
//
// Capture is one wrapper, Backend, around library.Backend: every UpdateBook /
// UpdateBooks through it diffs the incoming book against the stored one and
// logs the difference under a fixed action name. Wrapping the backend instead
// of instrumenting each handler means a new write path is audited by being
// given the wrapper, not by remembering to add a call.
//
// Reading progress synced from devices is deliberately NOT routed through it
// (see internal/sync): it changes constantly and is not a pipeline action.
package audit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/log"
	"github.com/duckpuppy/comic-server/internal/workflow"
)

// Retention is how long entries are kept before PruneOld removes them.
const Retention = 180 * 24 * time.Hour

// Action names recorded in the log.
const (
	ActionConvertCBZ        = "convert_cbz"
	ActionWriteMetadata     = "write_metadata"
	ActionScanInfo          = "scan_info"
	ActionDataManager       = "data_manager"
	ActionOrganize          = "organize"
	ActionScrape            = "scrape"
	ActionLinkWanted        = "link_wanted"
	ActionRollback          = "rollback"
	ActionUndo              = "undo"
	ActionDuplicateTrash    = "duplicate_trash"
	ActionVariantCovers     = "variant_covers"
	ActionWantedFromMissing = "wanted_from_missing"
	ActionFormatDetect      = "format_detect"
	ActionCountFill         = "count_fill"
	ActionAcceptFlag        = "accept_flag"
	ActionPhysicalAdd       = "physical_add"
	ActionPageManager       = "page_manager"
	ActionRelink            = "relink"
	ActionRemoveRecord      = "remove_record"
	ActionCVNoMatch         = "cv_nomatch"
	ActionCVMatch           = "cv_match"
	ActionEdit              = "edit"
	ActionReadingList       = "reading_list"
)

// Change is one field's before/after.
type Change struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// stageKey is the custom value holding the workflow stage. It is reported as
// stage_before/stage_after on the entry, not as a field change.
const stageKey = "comic_server_workflow_stage"

// ignoredFields never show up as changes: they are bookkeeping, not data.
var ignoredFields = map[string]bool{
	"CustomValuesStore": true, // diffed per key below
}

// Diff lists the fields that differ between before and after. Scalar fields
// are compared by value; custom values per key; slice fields (page lists
// etc.) are reported as a count change.
func Diff(before, after *library.ComicBook) []Change {
	var out []Change
	bv, av := reflect.ValueOf(*before), reflect.ValueOf(*after)
	t := bv.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || ignoredFields[f.Name] {
			continue
		}
		b, a := bv.Field(i), av.Field(i)
		if reflect.DeepEqual(b.Interface(), a.Interface()) {
			continue
		}
		switch b.Kind() {
		case reflect.Slice, reflect.Map:
			out = append(out, Change{f.Name, fmt.Sprintf("%d items", b.Len()), fmt.Sprintf("%d items", a.Len())})
		default:
			out = append(out, Change{f.Name, scalar(b), scalar(a)})
		}
	}

	bc, ac := library.CustomValuesMap(before.CustomValuesStore), library.CustomValuesMap(after.CustomValuesStore)
	keys := map[string]bool{}
	for k := range bc {
		keys[k] = true
	}
	for k := range ac {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		if k != stageKey && bc[k] != ac[k] {
			sorted = append(sorted, k)
		}
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		out = append(out, Change{"custom:" + k, bc[k], ac[k]})
	}
	return out
}

// FieldValue returns the book's field as the audit diff would record it, so a
// current value can be compared with an entry's old/new text. ok is false
// for an unknown field name.
func FieldValue(book *library.ComicBook, name string) (string, bool) {
	v := reflect.ValueOf(*book).FieldByName(name)
	if !v.IsValid() {
		return "", false
	}
	return scalar(v), true
}

func scalar(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	}
	if ct, ok := v.Interface().(library.ComicTime); ok {
		if ct.IsZero() {
			return ""
		}
		return ct.Format(time.RFC3339)
	}
	return fmt.Sprint(v.Interface())
}

// Sink is where entries go. *configdb.DB satisfies it.
type Sink interface {
	AppendAudit(rows []configdb.AuditRow) error
}

// Backend wraps a library.Backend and logs every update made through it
// under Action. Reads and everything else pass straight through.
type Backend struct {
	library.Backend
	Action string
	Sink   Sink
}

// Wrap returns b unchanged when there is no sink to write to.
func Wrap(b library.Backend, action string, sink Sink) library.Backend {
	if sink == nil {
		return b
	}
	return &Backend{Backend: b, Action: action, Sink: sink}
}

// GetBook returns a COPY of the stored book. The XML backend hands out a
// pointer into the live library, so a caller that edits it in place would
// change the stored record before UpdateBooks runs and the diff would come up
// empty. Callers that go through this wrapper get a private copy to edit;
// the stored record only changes when they call UpdateBook(s).
func (a *Backend) GetBook(id string) (*library.ComicBook, error) {
	book, err := a.Backend.GetBook(id)
	if err != nil || book == nil {
		return book, err
	}
	cp := *book
	return &cp, nil
}

// DeleteBook removes the record and logs that it happened.
func (a *Backend) DeleteBook(id string) error {
	old, _ := a.Backend.GetBook(id)
	if err := a.Backend.DeleteBook(id); err != nil {
		return err
	}
	row := configdb.AuditRow{
		At: time.Now(), BookID: id, Action: a.Action,
		Changes: `[{"field":"record","old":"present","new":"deleted"}]`,
	}
	if old != nil {
		row.Series, row.Number = old.Series, old.Number
		row.StageBefore = workflow.GetStage(old).String()
	}
	Record(a.Sink, row)
	return nil
}

// UpdateBook logs then applies a single update.
func (a *Backend) UpdateBook(book *library.ComicBook) error {
	return a.UpdateBooks([]*library.ComicBook{book})
}

// UpdateBooks computes diffs against the stored books, applies the update,
// and only records entries once the update succeeded. A failure to write the
// audit log is logged but never fails the pipeline action itself.
func (a *Backend) UpdateBooks(books []*library.ComicBook) error {
	rows := a.rowsFor(books)
	if err := a.Backend.UpdateBooks(books); err != nil {
		return err
	}
	if len(rows) > 0 {
		if err := a.Sink.AppendAudit(rows); err != nil {
			log.Warn().Err(err).Str("action", a.Action).Msg("Failed to write audit log")
		}
	}
	return nil
}

func (a *Backend) rowsFor(books []*library.ComicBook) []configdb.AuditRow {
	now := time.Now()
	var rows []configdb.AuditRow
	for _, nb := range books {
		old, err := a.Backend.GetBook(nb.ID)
		if err != nil || old == nil {
			continue
		}
		changes := Diff(old, nb)
		sb, sa := workflow.GetStage(old).String(), workflow.GetStage(nb).String()
		if len(changes) == 0 && sb == sa {
			continue
		}
		data, _ := json.Marshal(changes)
		rows = append(rows, configdb.AuditRow{
			At: now, BookID: nb.ID, Action: a.Action, Series: nb.Series, Number: nb.Number,
			StageBefore: sb, StageAfter: sa, Changes: string(data),
		})
	}
	return rows
}

// Record writes an entry for an action that is not a backend update (for
// example a file being quarantined).
func Record(sink Sink, row configdb.AuditRow) {
	if sink == nil {
		return
	}
	if row.At.IsZero() {
		row.At = time.Now()
	}
	if err := sink.AppendAudit([]configdb.AuditRow{row}); err != nil {
		log.Warn().Err(err).Str("action", row.Action).Msg("Failed to write audit log")
	}
}
