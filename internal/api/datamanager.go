package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/duckpuppy/comic-server/internal/configdb"
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

// DMRunResult is the response for both preview and apply - Applied is
// false for a preview (nothing was written) and true once a run has
// actually been committed via the backend.
type DMRunResult struct {
	Processed int            `json:"processed"`
	Changed   int            `json:"changed"`
	Applied   bool           `json:"applied"`
	Books     []DMBookChange `json:"books"`
	Errors    []string       `json:"errors,omitempty"`
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
	s.runDataManager(w, r, "/datamanager-preview", false)
}

// handleDataManagerApply does the same full rule run as
// handleDataManagerPreview, then commits every changed book in one action
// via Backend.UpdateBooks - matching the original plugin's own
// whole-run-at-once behavior (no per-book cherry-picking).
// POST /api/library/lists/:listId/datamanager-apply
func (s *Server) handleDataManagerApply(w http.ResponseWriter, r *http.Request) {
	s.runDataManager(w, r, "/datamanager-apply", true)
}

func (s *Server) runDataManager(w http.ResponseWriter, r *http.Request, suffix string, apply bool) {
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

	rulesets, err := loadEnabledDMRulesets(s.configDB)
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

	result := DMRunResult{Processed: len(books), Applied: apply}
	var toUpdate []*library.ComicBook

	for _, book := range books {
		// Always work on a copy, never the pointer MatchBooks returned -
		// that pointer may be shared with the backend's cached library
		// snapshot (see SQLiteBackend.cachedLibrary's own doc comment on
		// why nothing may mutate a cached book in place).
		working := *book
		changes, err := datamanager.ApplyAll(&working, rulesets)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", book.ID, err))
		}
		if len(changes) == 0 {
			continue
		}

		wireChanges := make([]DMFieldChange, len(changes))
		for i, c := range changes {
			wireChanges[i] = DMFieldChange{Field: c.Field, Custom: c.Custom, Old: c.Old, New: c.New}
		}
		result.Books = append(result.Books, DMBookChange{
			BookID:  book.ID,
			Series:  book.Series,
			Number:  book.Number,
			Title:   book.Title,
			Changes: wireChanges,
		})
		result.Changed++

		if apply {
			toUpdate = append(toUpdate, &working)
		}
	}

	if apply && len(toUpdate) > 0 {
		if err := s.backend.UpdateBooks(toUpdate); err != nil {
			log.Error().Err(err).Msg("Failed to save Data Manager rule run")
			result.Errors = append(result.Errors, err.Error())
		}
	}

	s.writeJSON(w, http.StatusOK, result)
}

func listIDFromSubPath(path, suffix string) string {
	s := strings.TrimPrefix(path, "/api/library/lists/")
	return strings.TrimSuffix(s, suffix)
}

// loadEnabledDMRulesets walks config.db's dm_groups/dm_rulesets in the
// same depth-first document order dataman.dat was imported in (both
// tables share one global sort_order sequence assigned during import - see
// comic-server-764.5's design notes - so merging children of each group by
// sort_order and recursing reproduces the original file's evaluation
// order exactly). Disabled groups and rulesets are skipped entirely: their
// own Disabled flag was already correctly set at import time from
// dataman.dat's <disabled> container, so no ancestor-walking is needed
// here.
func loadEnabledDMRulesets(db *configdb.DB) ([]datamanager.Ruleset, error) {
	var out []datamanager.Ruleset
	if err := walkDMGroup(db, "", &out); err != nil {
		return nil, err
	}
	return out, nil
}

type dmSortable struct {
	order   int
	group   *configdb.DMGroup
	ruleset *configdb.DMRuleset
}

func walkDMGroup(db *configdb.DB, groupID string, out *[]datamanager.Ruleset) error {
	groups, err := db.ListDMGroups(groupID)
	if err != nil {
		return fmt.Errorf("list dm_groups under %q: %w", groupID, err)
	}
	rulesets, err := db.ListDMRulesets(groupID)
	if err != nil {
		return fmt.Errorf("list dm_rulesets under %q: %w", groupID, err)
	}

	items := make([]dmSortable, 0, len(groups)+len(rulesets))
	for i := range groups {
		items = append(items, dmSortable{order: groups[i].SortOrder, group: &groups[i]})
	}
	for i := range rulesets {
		items = append(items, dmSortable{order: rulesets[i].SortOrder, ruleset: &rulesets[i]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].order < items[j].order })

	for _, it := range items {
		switch {
		case it.group != nil:
			if it.group.Disabled {
				continue
			}
			if err := walkDMGroup(db, it.group.ID, out); err != nil {
				return err
			}
		case it.ruleset != nil:
			if it.ruleset.Disabled {
				continue
			}
			rs, err := loadDMRuleset(db, it.ruleset)
			if err != nil {
				return err
			}
			*out = append(*out, rs)
		}
	}
	return nil
}

func loadDMRuleset(db *configdb.DB, rec *configdb.DMRuleset) (datamanager.Ruleset, error) {
	dbRules, err := db.ListDMRules(rec.ID)
	if err != nil {
		return datamanager.Ruleset{}, fmt.Errorf("list dm_rules for ruleset %s: %w", rec.ID, err)
	}
	dbActions, err := db.ListDMActions(rec.ID)
	if err != nil {
		return datamanager.Ruleset{}, fmt.Errorf("list dm_actions for ruleset %s: %w", rec.ID, err)
	}

	rules := make([]datamanager.Rule, len(dbRules))
	for i, r := range dbRules {
		rules[i] = datamanager.Rule{Field: r.Field, Modifier: r.Modifier, Value: r.Value}
	}
	actions := make([]datamanager.Action, len(dbActions))
	for i, a := range dbActions {
		actions[i] = datamanager.Action{Field: a.Field, Modifier: a.Modifier, Value: a.Value}
	}

	return datamanager.Ruleset{
		Name:    rec.Name,
		Mode:    rec.Mode,
		Rules:   rules,
		Actions: actions,
	}, nil
}
