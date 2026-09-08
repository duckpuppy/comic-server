package datamanager

import (
	"fmt"
	"sort"

	"github.com/duckpuppy/comic-server/internal/configdb"
)

// LoadRulesets walks config.db's dm_groups/dm_rulesets in the same
// depth-first document order dataman.dat was imported in (both tables
// share one global sort_order sequence assigned during import - see
// comic-server-764.5's design notes - so merging children of each group by
// sort_order and recursing reproduces the original file's evaluation
// order exactly). Disabled groups and rulesets are skipped entirely: their
// own Disabled flag was already correctly set at import time from
// dataman.dat's <disabled> container, so no ancestor-walking is needed
// here.
//
// Moved here from internal/api (comic-server-1iv.1) so internal/workflow
// can load the same rule set for its own idempotent-preview stage check,
// without either package depending on the other.
func LoadRulesets(db *configdb.DB) ([]Ruleset, error) {
	var out []Ruleset
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

func walkDMGroup(db *configdb.DB, groupID string, out *[]Ruleset) error {
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

func loadDMRuleset(db *configdb.DB, rec *configdb.DMRuleset) (Ruleset, error) {
	dbRules, err := db.ListDMRules(rec.ID)
	if err != nil {
		return Ruleset{}, fmt.Errorf("list dm_rules for ruleset %s: %w", rec.ID, err)
	}
	dbActions, err := db.ListDMActions(rec.ID)
	if err != nil {
		return Ruleset{}, fmt.Errorf("list dm_actions for ruleset %s: %w", rec.ID, err)
	}

	rules := make([]Rule, len(dbRules))
	for i, r := range dbRules {
		rules[i] = Rule{Field: r.Field, Modifier: r.Modifier, Value: r.Value}
	}
	actions := make([]Action, len(dbActions))
	for i, a := range dbActions {
		actions[i] = Action{Field: a.Field, Modifier: a.Modifier, Value: a.Value}
	}

	return Ruleset{
		Name:    rec.Name,
		Mode:    rec.Mode,
		Rules:   rules,
		Actions: actions,
	}, nil
}
