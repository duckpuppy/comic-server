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

// GetCustomValue reads key from a CustomValuesStore string, returning
// ok=false if the key isn't present. Several packages previously rolled
// their own version of this read (internal/datamanager's getCustomValue,
// internal/comicvine's extractCVVolumeID/extractCVIssueID) - this is the
// shared, exported counterpart to SetCustomValue, added for
// internal/workflow (comic-server-1iv.1) rather than adding a fourth
// private copy.
func GetCustomValue(store, key string) (string, bool) {
	for _, p := range splitCustomValues(store) {
		k, v, ok := strings.Cut(p, "=")
		if ok && k == key {
			return v, true
		}
	}
	return "", false
}

// RemoveCustomValue drops key from a CustomValuesStore string, keeping the
// other keys. A key that is not present leaves the store unchanged.
func RemoveCustomValue(store, key string) string {
	pairs := splitCustomValues(store)
	kept := pairs[:0:0]
	for _, p := range pairs {
		if k, _, ok := strings.Cut(p, "="); ok && k == key {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) == len(pairs) {
		return store
	}
	return joinCustomValues(kept)
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

// CustomValuesMap parses a CustomValuesStore string into a key -> value map.
func CustomValuesMap(store string) map[string]string {
	out := map[string]string{}
	for _, p := range splitCustomValues(store) {
		if k, v, ok := strings.Cut(p, "="); ok {
			out[k] = v
		}
	}
	return out
}
