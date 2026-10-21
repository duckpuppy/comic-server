package api

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/duckpuppy/comic-server/internal/comicvine"
	"github.com/duckpuppy/comic-server/internal/library"
)

// Autonumber (tracker #30): number the books of a bulk edit 1, 2, 3... in a
// chosen order. Unlike the other bulk edits it needs the whole scope at once
// (a book's number depends on its place in the list), so the handler works
// out one number per book first and then applies it as a plain Number edit.

// Autonumber orders.
const (
	autonumberOrderCurrent = "current" // the order the scope came in (Browse order or ticked order)
	autonumberOrderDate    = "date"    // Year, Month, Day; books with no date go last
	autonumberOrderTitle   = "title"
	autonumberOrderFile    = "file" // file name, numbers compared as numbers
)

const maxAutonumberPad = 10

// Autonumber is the autonumber part of a BookEditRequest. It only sets Number.
type Autonumber struct {
	Order string `json:"order"`
	Start int    `json:"start"`
	Step  int    `json:"step"`
	// Pad is the minimum width, filled with zeros on the left (0 = none).
	Pad int `json:"pad,omitempty"`
}

// validate fills the default step and refuses values that make no sense.
func (a *Autonumber) validate() error {
	switch a.Order {
	case autonumberOrderCurrent, autonumberOrderDate, autonumberOrderTitle, autonumberOrderFile:
	default:
		return fmt.Errorf("unknown autonumber order %q", a.Order)
	}
	if a.Step == 0 {
		a.Step = 1
	}
	if a.Step < 1 {
		return fmt.Errorf("the step must be 1 or more")
	}
	if a.Start < 0 {
		return fmt.Errorf("the start must be 0 or more")
	}
	if a.Pad < 0 || a.Pad > maxAutonumberPad {
		return fmt.Errorf("padding must be between 0 and %d", maxAutonumberPad)
	}
	return nil
}

func (a Autonumber) format(i int) string {
	s := strconv.Itoa(a.Start + i*a.Step)
	if len(s) < a.Pad {
		s = strings.Repeat("0", a.Pad-len(s)) + s
	}
	return s
}

// autonumberSkip is a book that keeps its number.
type autonumberSkip struct {
	ID     string `json:"id"`
	Series string `json:"series"`
	Number string `json:"number"`
	Reason string `json:"reason"`
}

// planAutonumber returns the new Number for each book it will number, and the
// books it leaves alone. A book that carries a ComicVine issue id is skipped:
// its number is part of the match to ComicVine, and a new one would make
// Missing Issues and the scraper misread it. books is in scope order.
func planAutonumber(books []*library.ComicBook, a Autonumber) (map[string]string, []autonumberSkip) {
	var todo []*library.ComicBook
	var skipped []autonumberSkip
	for _, b := range books {
		if comicvine.ExtractCVIssueID(b.CustomValuesStore) > 0 {
			skipped = append(skipped, autonumberSkip{ID: b.ID, Series: b.Series, Number: b.Number, Reason: "has a ComicVine issue"})
			continue
		}
		todo = append(todo, b)
	}
	// Stable, so books that compare equal (and "current") keep scope order.
	sort.SliceStable(todo, func(i, j int) bool { return autonumberLess(a.Order, todo[i], todo[j]) })
	out := make(map[string]string, len(todo))
	for i, b := range todo {
		out[b.ID] = a.format(i)
	}
	return out, skipped
}

func autonumberLess(order string, x, y *library.ComicBook) bool {
	switch order {
	case autonumberOrderDate:
		dx, dy := dateKey(x), dateKey(y)
		if (dx == 0) != (dy == 0) {
			return dx != 0 // undated books go last
		}
		return dx < dy
	case autonumberOrderTitle:
		return naturalLess(x.Title, y.Title)
	case autonumberOrderFile:
		return naturalLess(filepath.Base(x.FilePath), filepath.Base(y.FilePath))
	}
	return false
}

func dateKey(b *library.ComicBook) int {
	if b.Year <= 0 {
		return 0
	}
	return b.Year*10000 + max(b.Month, 0)*100 + max(b.Day, 0)
}

// naturalLess compares case-insensitively with runs of digits compared as
// numbers, so "Issue 2" comes before "Issue 10".
func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		if unicode.IsDigit(rune(a[0])) && unicode.IsDigit(rune(b[0])) {
			na, ra := digitRun(a)
			nb, rb := digitRun(b)
			if na != nb {
				return len(na) < len(nb) || (len(na) == len(nb) && na < nb)
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

// digitRun splits the leading digits off s, without leading zeros.
func digitRun(s string) (digits, rest string) {
	i := 0
	for i < len(s) && unicode.IsDigit(rune(s[i])) {
		i++
	}
	return strings.TrimLeft(s[:i], "0"), s[i:]
}
