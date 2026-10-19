// Package collects holds the links from a collected edition (TPB, hardcover,
// omnibus, event book) to the issues it contains (tracker #36, #38).
//
// A collected book keeps its own identity (series, number, ComicVine tag);
// the issues it contains are a separate list stored in one custom value. The
// list is local to comic-server: ComicVine cannot be edited, and ComicRack
// ignores comic_server_ values. One entry is one run of issues in one series,
// so a book from one series holds one entry and an event book holds several.
package collects

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// Key is the custom value holding the links. The comic_server_ prefix keeps
// it out of the metadata fingerprint.
const Key = "comic_server_collects"

// Entry kinds.
const (
	KindMain   = "main"   // the story the book is about
	KindBackup = "backup" // a backup story from another issue
)

// Entry sources.
const (
	SourceManual    = "manual"
	SourceSuggested = "suggested"
)

// Limits keep a bad request from bloating a book record.
const (
	MaxEntries  = 200
	maxFieldLen = 200
)

// Entry is a run of issues of one series that a collected book contains.
type Entry struct {
	// Volume is the ComicVine volume id; 0 when not known.
	Volume int `json:"volume,omitempty"`
	// Series is a name hint, shown when there is no volume id and kept as a
	// label when there is one.
	Series string `json:"series,omitempty"`
	// From and To are issue numbers as text ("1", "12.1", "Annual 1"). To is
	// empty for a single issue.
	From string `json:"from"`
	To   string `json:"to,omitempty"`
	// Kind is KindMain (default) or KindBackup.
	Kind string `json:"kind,omitempty"`
	// Partial marks a story taken from an issue, not the whole issue.
	Partial bool `json:"partial,omitempty"`
	// Source is SourceManual (default) or SourceSuggested.
	Source string `json:"source,omitempty"`
}

// Validate checks one entry and returns a readable reason when it is bad.
func (e Entry) Validate() error {
	if e.Volume < 0 {
		return fmt.Errorf("volume must not be negative")
	}
	if e.Volume == 0 && strings.TrimSpace(e.Series) == "" {
		return fmt.Errorf("an entry needs a ComicVine volume or a series name")
	}
	if strings.TrimSpace(e.From) == "" {
		return fmt.Errorf("an entry needs a first issue number")
	}
	for _, f := range []string{e.Series, e.From, e.To} {
		if len(f) > maxFieldLen {
			return fmt.Errorf("a field is longer than %d characters", maxFieldLen)
		}
	}
	switch e.Kind {
	case "", KindMain, KindBackup:
	default:
		return fmt.Errorf("unknown kind %q", e.Kind)
	}
	switch e.Source {
	case "", SourceManual, SourceSuggested:
	default:
		return fmt.Errorf("unknown source %q", e.Source)
	}
	return nil
}

// Normalize trims text and fills the defaults, so equal links compare equal.
func (e Entry) Normalize() Entry {
	e.Series = strings.TrimSpace(e.Series)
	e.From = strings.TrimSpace(e.From)
	e.To = strings.TrimSpace(e.To)
	if e.To == e.From {
		e.To = ""
	}
	if e.Kind == "" {
		e.Kind = KindMain
	}
	if e.Source == "" {
		e.Source = SourceManual
	}
	return e
}

// ValidateAll checks a whole list.
func ValidateAll(entries []Entry) error {
	if len(entries) > MaxEntries {
		return fmt.Errorf("at most %d entries", MaxEntries)
	}
	for i, e := range entries {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("entry %d: %w", i+1, err)
		}
	}
	return nil
}

// Encode turns entries into the stored text. Custom values use ',' and '='
// as separators, so every field is percent-escaped; entries are joined by '+'
// and fields by '|' (both escaped inside a field).
func Encode(entries []Entry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		e = e.Normalize()
		partial := "0"
		if e.Partial {
			partial = "1"
		}
		vol := ""
		if e.Volume > 0 {
			vol = fmt.Sprint(e.Volume)
		}
		parts = append(parts, strings.Join([]string{
			vol, esc(e.Series), esc(e.From), esc(e.To),
			e.Kind, partial, e.Source,
		}, "|"))
	}
	return strings.Join(parts, "+")
}

// esc percent-escapes everything but letters, digits and ._-, so a field can
// never hold a custom-value separator (',', '=') or ours ('+', '|', space).
func esc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// Decode reads the stored text. A malformed entry is skipped, never an error:
// a damaged value must not stop a book page from opening.
func Decode(s string) []Entry {
	var out []Entry
	for _, part := range strings.Split(s, "+") {
		f := strings.Split(part, "|")
		if len(f) != 7 {
			continue
		}
		var e Entry
		if f[0] != "" {
			if _, err := fmt.Sscanf(f[0], "%d", &e.Volume); err != nil {
				continue
			}
		}
		var err error
		if e.Series, err = url.PathUnescape(f[1]); err != nil {
			continue
		}
		if e.From, err = url.PathUnescape(f[2]); err != nil {
			continue
		}
		if e.To, err = url.PathUnescape(f[3]); err != nil {
			continue
		}
		e.Kind, e.Partial, e.Source = f[4], f[5] == "1", f[6]
		out = append(out, e.Normalize())
	}
	return out
}

// Get returns the links stored on b (nil when none).
func Get(b *library.ComicBook) []Entry {
	v, _ := library.GetCustomValue(b.CustomValuesStore, Key)
	if v == "" {
		return nil
	}
	return Decode(v)
}

// Has reports whether b carries any collects link.
func Has(b *library.ComicBook) bool {
	v, _ := library.GetCustomValue(b.CustomValuesStore, Key)
	return v != ""
}

// Set replaces the links on b; an empty list removes the value. Returns
// whether the stored text changed. It does not validate: call ValidateAll.
func Set(b *library.ComicBook, entries []Entry) bool {
	old, _ := library.GetCustomValue(b.CustomValuesStore, Key)
	if len(entries) == 0 {
		if old == "" {
			return false
		}
		b.CustomValuesStore = library.RemoveCustomValue(b.CustomValuesStore, Key)
		return true
	}
	enc := Encode(entries)
	if enc == old {
		return false
	}
	b.CustomValuesStore = library.SetCustomValue(b.CustomValuesStore, Key, enc)
	return true
}
