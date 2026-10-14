package library

import "strings"

// CollectionStatusPhysical is the BookCollectionStatus value that marks a
// fileless book as a paper copy the user owns (comic-server-vffq). Any other
// fileless book is a Wanted placeholder. ComicRack's own field is used so the
// mark shows in ComicRack, matches in smart lists and survives export.
const CollectionStatusPhysical = "Physical"

// IsPhysical reports whether b is a paper copy: no file, and Collection
// Status "Physical" (any case).
func IsPhysical(b *ComicBook) bool {
	return b.FilePath == "" && strings.EqualFold(strings.TrimSpace(b.BookCollectionStatus), CollectionStatusPhysical)
}

// IsWantedPlaceholder reports whether b is a Wanted placeholder: no file and
// not a paper copy.
func IsWantedPlaceholder(b *ComicBook) bool {
	return b.FilePath == "" && !IsPhysical(b)
}
