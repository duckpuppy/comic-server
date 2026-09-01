package datamanager

import (
	"encoding/xml"
	"fmt"
	"io"
)

// xmlRule/xmlAction/xmlRuleset/xmlGroup mirror dataman.dat's real element
// shapes, ground-truthed against the user's actual 343KB/595-ruleset file
// (see comic-server-764's design notes) - not the Custom(...) wrapper
// claim from an unreliable earlier research pass.
type xmlRule struct {
	Field    string `xml:"field,attr"`
	Modifier string `xml:"modifier,attr"`
	Value    string `xml:"value,attr"`
}

type xmlAction struct {
	Field    string `xml:"field,attr"`
	Modifier string `xml:"modifier,attr"`
	Value    string `xml:"value,attr"`
}

type xmlRuleset struct {
	Name        string      `xml:"name,attr"`
	Comment     string      `xml:"comment,attr"`
	RulesetMode string      `xml:"rulesetmode,attr"`
	Rules       []xmlRule   `xml:"rule"`
	Actions     []xmlAction `xml:"action"`
}

// xmlGroup is recursive (<group> nests <group>) and can hold rulesets
// directly alongside child groups, in any document order - order must be
// captured, not assumed, since it's what depth-first evaluation replay
// depends on.
type xmlGroup struct {
	Name     string     `xml:"name,attr"`
	Comment  string     `xml:"comment,attr"`
	Children []xmlChild `xml:",any"`
}

// xmlChild captures one child element of a <group> or <disabled> in
// document order, decoded generically so <group> and <ruleset> children
// can be interleaved and processed depth-first: dataman.dat's own
// evaluation order fully processes each child group (including its
// nested rulesets) before moving to the container's own top-level
// rulesets, which happens naturally by walking Children in order.
type xmlChild struct {
	XMLName xml.Name
	Group   *xmlGroup   `xml:"-"`
	Ruleset *xmlRuleset `xml:"-"`
}

func (c *xmlChild) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	c.XMLName = start.Name
	switch start.Name.Local {
	case "group":
		var g xmlGroup
		if err := d.DecodeElement(&g, &start); err != nil {
			return err
		}
		c.Group = &g
	case "ruleset":
		var rs xmlRuleset
		if err := d.DecodeElement(&rs, &start); err != nil {
			return err
		}
		c.Ruleset = &rs
	default:
		// filtersanddefaults and anything else unrecognized - skip.
		if err := d.Skip(); err != nil {
			return err
		}
	}
	return nil
}

type xmlCollection struct {
	Name     string     `xml:"name,attr"`
	Version  string     `xml:"version,attr"`
	Groups   []xmlGroup `xml:"group"`
	Disabled *xmlGroup  `xml:"disabled"`
}

// ImportedGroup is one parsed group node, ready for a caller to persist
// (e.g. via configdb.DB.CreateDMGroup) - flattened out of the recursive
// xmlGroup tree so the importer doesn't need to know about configdb, and
// configdb doesn't need to know about XML.
type ImportedGroup struct {
	ID        string // caller-assigned before Rulesets reference it
	ParentID  string
	Name      string
	Comment   string
	Disabled  bool
	SortOrder int
}

// ImportedRuleset is one parsed ruleset, with its own Rules/Actions
// already attached and its GroupID pointing at the ImportedGroup it
// belongs to (empty for a top-level ruleset).
type ImportedRuleset struct {
	GroupID   string
	Name      string
	Comment   string
	Mode      string
	Disabled  bool
	SortOrder int
	Rules     []Rule
	Actions   []Action
}

// ImportResult is the full flattened result of parsing a dataman.dat file,
// in depth-first document order - ready to hand to configdb in a single
// pass without the caller needing to walk a tree itself.
type ImportResult struct {
	CollectionName string
	Groups         []ImportedGroup
	Rulesets       []ImportedRuleset
}

// nextID is a caller-suppliable ID generator, so callers can inject a
// deterministic sequence in tests instead of pulling in a UUID dependency
// here - internal/datamanager has no persistence concerns of its own.
type nextID func() string

// ParseDataman parses a dataman.dat file's contents into a flat
// ImportResult, walking groups and rulesets in the same depth-first
// document order the real plugin evaluates them in (see comic-server-764's
// design notes: a later ruleset's action can overwrite an earlier one's on
// the same field, so preserving this order is not cosmetic).
func ParseDataman(r io.Reader, genID nextID) (*ImportResult, error) {
	var coll xmlCollection
	if err := xml.NewDecoder(r).Decode(&coll); err != nil {
		return nil, fmt.Errorf("parse dataman.dat: %w", err)
	}

	res := &ImportResult{CollectionName: coll.Name}
	sortOrder := 0

	var walk func(children []xmlChild, parentID string, disabled bool)
	walk = func(children []xmlChild, parentID string, disabled bool) {
		for _, child := range children {
			switch {
			case child.Group != nil:
				g := child.Group
				id := genID()
				order := sortOrder
				sortOrder++
				res.Groups = append(res.Groups, ImportedGroup{
					ID:        id,
					ParentID:  parentID,
					Name:      g.Name,
					Comment:   g.Comment,
					Disabled:  disabled,
					SortOrder: order,
				})
				walk(g.Children, id, disabled)
			case child.Ruleset != nil:
				rs := child.Ruleset
				order := sortOrder
				sortOrder++
				mode := rs.RulesetMode
				if mode == "" {
					mode = "AND"
				}
				res.Rulesets = append(res.Rulesets, ImportedRuleset{
					GroupID:   parentID,
					Name:      rs.Name,
					Comment:   rs.Comment,
					Mode:      mode,
					Disabled:  disabled,
					SortOrder: order,
					Rules:     convertRules(rs.Rules),
					Actions:   convertActions(rs.Actions),
				})
			}
		}
	}

	// Top-level <group> elements are direct children of <collection>, not
	// wrapped in an xmlChild - handle them as their own pass, in document
	// order relative to each other, before <disabled> (which always comes
	// last in the real file).
	for _, g := range coll.Groups {
		id := genID()
		order := sortOrder
		sortOrder++
		res.Groups = append(res.Groups, ImportedGroup{
			ID:        id,
			Name:      g.Name,
			Comment:   g.Comment,
			SortOrder: order,
		})
		walk(g.Children, id, false)
	}

	if coll.Disabled != nil {
		// <disabled> is a flag holder, not a real folder in the hierarchy -
		// it has no counterpart <group> element (dataman.dat's own group
		// count excludes it), so its direct children become top-level
		// groups/rulesets with Disabled propagated down, rather than
		// getting a synthetic group row of their own.
		walk(coll.Disabled.Children, "", true)
	}

	return res, nil
}

func convertRules(in []xmlRule) []Rule {
	out := make([]Rule, len(in))
	for i, r := range in {
		out[i] = Rule{Field: r.Field, Modifier: r.Modifier, Value: r.Value}
	}
	return out
}

func convertActions(in []xmlAction) []Action {
	out := make([]Action, len(in))
	for i, a := range in {
		out[i] = Action{Field: a.Field, Modifier: a.Modifier, Value: a.Value}
	}
	return out
}
