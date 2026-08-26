package config

import (
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ResolveSmartList finds a smart list by name and returns its GUID
// Returns error if list not found or multiple lists match
func ResolveSmartList(lib *library.ComicLibrary, name string) (string, string, error) {
	var matches []library.ComicListItem

	for _, list := range lib.ComicLists {
		// Only consider smart lists
		if !strings.Contains(list.Type, "SmartList") {
			continue
		}

		if list.Name == name {
			matches = append(matches, list)
		}
	}

	if len(matches) == 0 {
		return "", "", fmt.Errorf("smart list %q not found in library", name)
	}

	if len(matches) > 1 {
		return "", "", fmt.Errorf("multiple smart lists named %q found (list names should be unique)", name)
	}

	// Return GUID (use ID field) and name
	return matches[0].ID, matches[0].Name, nil
}

// FindListByGUID finds a smart list in the library by its GUID
// Returns nil if not found
func FindListByGUID(lib *library.ComicLibrary, guid string) *library.ComicListItem {
	for i := range lib.ComicLists {
		list := &lib.ComicLists[i]
		if list.ID == guid && strings.Contains(list.Type, "SmartList") {
			return list
		}
	}
	return nil
}
