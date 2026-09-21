package config

import (
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// isAssignableListType reports whether a list.Type has real book
// membership that can be resolved via GetBooksForList and assigned to a
// device - smart lists, ID lists, and reading lists (including CBL
// imports). Folders don't: they group other lists, not a set of books
// themselves. Mirrors handleDeviceListAdd's identical relaxation
// (internal/api/api.go, 2026-08-26) - that fix covered the REST API path
// device assignment actually uses; these CLI-only resolvers
// (ResolveSmartList/FindListByGUID, used by `config add-list`/
// `set-options`/`remove-list`) were missed at the time and still
// rejected a reading list by name/GUID until comic-server-hmld found it.
func isAssignableListType(t string) bool {
	return !strings.Contains(t, "Folder")
}

// ResolveSmartList finds an assignable list (smart list, ID list, or
// reading list - see isAssignableListType) by name and returns its GUID.
// The name is historical (this originally only matched smart lists);
// kept to avoid an unrelated rename across every CLI call site.
// Returns error if list not found or multiple lists match.
func ResolveSmartList(lib *library.ComicLibrary, name string) (string, string, error) {
	var matches []library.ComicListItem

	for _, list := range lib.ComicLists {
		if !isAssignableListType(list.Type) {
			continue
		}

		if list.Name == name {
			matches = append(matches, list)
		}
	}

	if len(matches) == 0 {
		return "", "", fmt.Errorf("list %q not found in library", name)
	}

	if len(matches) > 1 {
		return "", "", fmt.Errorf("multiple lists named %q found (list names should be unique)", name)
	}

	// Return GUID (use ID field) and name
	return matches[0].ID, matches[0].Name, nil
}

// FindListByGUID finds an assignable list (see isAssignableListType) in
// the library by its GUID. Returns nil if not found.
func FindListByGUID(lib *library.ComicLibrary, guid string) *library.ComicListItem {
	for i := range lib.ComicLists {
		list := &lib.ComicLists[i]
		if list.ID == guid && isAssignableListType(list.Type) {
			return list
		}
	}
	return nil
}
