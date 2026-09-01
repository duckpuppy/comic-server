package datamanager

// Rule is one condition of a Ruleset - a field/modifier/value triple
// matching dataman.dat's <rule field=".." modifier=".." value=".." />
// element. Multi-parameter modifiers (IsAnyOf, ContainsAnyOf, Range, ...)
// carry their extra parameters "||"-joined inside Value, exactly as
// dataman.dat stores them (see comic-server-764's design notes).
type Rule struct {
	Field    string
	Modifier string
	Value    string
}

// Action is one write performed when a Ruleset's rules match, matching
// dataman.dat's <action field=".." modifier=".." value=".." /> element.
type Action struct {
	Field    string
	Modifier string
	Value    string
}

// Ruleset is a flat AND/OR-combined list of conditions plus the actions
// to apply when they match - matches dataman.dat's <ruleset> element.
// Rulesets are flat (single Mode for the whole Rules list) - nesting only
// happens at the Group level, never within one ruleset's own rules (see
// comic-server-764's design notes, confirmed against the user's real
// dataman.dat).
type Ruleset struct {
	Name    string
	Mode    string // "AND" or "OR"
	Rules   []Rule
	Actions []Action
}
