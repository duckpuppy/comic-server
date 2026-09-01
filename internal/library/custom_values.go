package library

import "strings"

// SetCustomValue adds or updates a key in a CustomValuesStore string
// (format: ",key1=value1,key2=value2"), preserving other keys. Shared
// between internal/comicvine (writing comicvine_volume/comicvine_issue)
// and internal/datamanager (writing custom-value fields like Concept,
// Concept Status, etc from rule actions) - see comic-server-764.2.
func SetCustomValue(store, key, value string) string {
	pairs := splitCustomValues(store)
	found := false
	for i, p := range pairs {
		k, _, ok := strings.Cut(p, "=")
		if ok && k == key {
			pairs[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		pairs = append(pairs, key+"="+value)
	}
	return joinCustomValues(pairs)
}

func splitCustomValues(store string) []string {
	var pairs []string
	for p := range strings.SplitSeq(store, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pairs = append(pairs, p)
	}
	return pairs
}

func joinCustomValues(pairs []string) string {
	if len(pairs) == 0 {
		return ""
	}
	return "," + strings.Join(pairs, ",")
}
