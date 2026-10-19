package collects

import (
	"fmt"
	"strconv"
	"strings"
)

// normNumber makes issue numbers comparable: "01" and "1" match, case is ignored.
func normNumber(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	if f, err := strconv.ParseFloat(n, 64); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return n
}

// Warnings checks one entry against the issue numbers of its volume and
// returns readable notes. numbers nil or empty means the volume is unknown
// to us: unknown is not a warning. Warnings never block a save.
func Warnings(e Entry, numbers []string) []string {
	var out []string
	e = e.Normalize()
	if e.To != "" {
		a, errA := strconv.ParseFloat(e.From, 64)
		b, errB := strconv.ParseFloat(e.To, 64)
		if errA == nil && errB == nil && a > b {
			out = append(out, fmt.Sprintf("the range is backwards (#%s to #%s)", e.From, e.To))
		}
	}
	if len(numbers) == 0 {
		return out
	}
	have := make(map[string]bool, len(numbers))
	for _, n := range numbers {
		have[normNumber(n)] = true
	}
	for _, n := range []string{e.From, e.To} {
		if n != "" && !have[normNumber(n)] {
			out = append(out, fmt.Sprintf("#%s is not in this ComicVine volume", n))
		}
	}
	return out
}
