package storage

import "github.com/duckpuppy/comic-server/internal/cbl"

// CBLImportSource records where an imported reading list came from
// (spec §4, comic-server-tnv4). Kept separate from library.ComicListItem
// deliberately - that struct round-trips through ComicDb.xml for real
// ComicRackCE compatibility, and CBL import provenance has no ComicRack
// equivalent to preserve there. Zero value means "not imported from a
// CBL" (the `lists.cbl_source` column is NULL for every ordinary list).
type CBLImportSource struct {
	// Source identifies where the CBL came from: "local_file" for a
	// direct upload, or "git:<repo-url>:<path>" for an entry fetched
	// from a cloned repo (e.g. DieselTech/CBL-ReadingLists) - see spec
	// §3/§5.
	Source string
	// SourceRef is the commit SHA (git sources) or content hash (local
	// file sources) this import was taken from - used by watch/reimport
	// (comic-server-zw0o, not yet implemented) to detect upstream
	// changes.
	SourceRef string
	// ImportedAt is when this list was last (re)imported, RFC3339.
	ImportedAt string
}

// CBLImportEntry is one row of cbl_import_entries: a single CBL <Book>
// entry as it was seen at import time, whether or not it matched a real
// book. Kept even for unmatched entries so the match-correction UI
// (comic-server-a2hz) and wanted-list integration (comic-server-sx2d)
// have something to act on without re-parsing the original CBL file.
type CBLImportEntry struct {
	ID       string
	ListID   string
	BookID   string // empty when unmatched (Path == cbl.MatchNone)
	Position int
	Path     cbl.MatchPath

	// Raw CBL entry fields, always populated regardless of match
	// outcome - see internal/cbl.Book.
	Series    string
	Number    string
	Volume    int
	Year      int
	Format    string
	CVIssueID int // 0 if the entry carried no <Database Name="cv"> id
}
