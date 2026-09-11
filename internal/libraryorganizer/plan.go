package libraryorganizer

import (
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// PlannedMove is one book's computed old->new path, before anything is
// written - comic-server-3bz.4's whole purpose: let the user see and
// approve every move before comic-server (for the first time ever) writes
// to the user's own existing directory structure.
type PlannedMove struct {
	BookID string
	Series string
	Number string
	Title  string

	// OldRawPath/NewRawPath are Windows-style library paths (matching
	// book.FilePath's own recorded style and the profile's BaseFolder) -
	// NOT yet resolved to a path this process can actually open. NewFolder
	// is the planned folder as individual segments (matching
	// MakeFolderPath's own return shape) purely for display; NewRawPath is
	// what actually gets used.
	OldRawPath string
	NewRawPath string
	NewFolder  []string
	NewFile    string

	// OldResolvedPath/NewResolvedPath are the same paths run through the
	// caller's resolvePath (see comic-server's existing
	// config.ResolveLibraryFilePath / cbzconvert.Convert's own resolvePath
	// parameter for the established pattern) - the paths this process
	// would actually read/write.
	OldResolvedPath string
	NewResolvedPath string

	// Skipped is true when the book didn't pass the profile's exclude
	// rules (comic-server-3bz.3) - included in the plan for visibility
	// (so a preview UI can show "excluded by rule X" rather than the book
	// just silently vanishing), but never a candidate to actually move.
	Skipped bool

	// Failed is true when the template couldn't fully resolve (an
	// unsupported or empty-required field - see comic-server-3bz.1's
	// InsertFieldsIntoTemplate ok=false case). FailReason explains why.
	Failed     bool
	FailReason string

	// Collision is true when NewResolvedPath already has a DIFFERENT file
	// at it - either something pre-existing on disk (via the caller's
	// fileExists check) or another book planned to the exact same
	// destination within this same run. Never true for a book whose
	// planned destination equals its own current path (a genuine no-op,
	// not a collision).
	Collision       bool
	CollisionReason string
}

// PlanOptions bundles Plan's non-book-specific inputs.
type PlanOptions struct {
	Profile        Profile
	Exclude        ExcludeConfig
	BaseFolder     string // Windows-style, e.g. `G:\Comics`
	FolderTemplate string
	FileTemplate   string

	// ResolvePath translates a raw Windows-style library path into a path
	// this process can actually open - same shape and purpose as
	// cbzconvert.Convert's resolvePath parameter (comic-server's existing
	// config.Config.ResolveLibraryFilePath). Pass nil to use raw paths
	// unchanged.
	ResolvePath func(string) string

	// FileExists checks whether a RESOLVED path already has something on
	// disk - injected rather than calling os.Stat directly so Plan stays
	// pure/no-file-I/O and fully testable, matching this issue's own
	// "plan every path... WITHOUT writing anything" scope. Pass nil to
	// skip collision detection against the real filesystem (still detects
	// collisions between two books planned to the same destination within
	// this run).
	FileExists func(resolvedPath string) bool
}

// Plan computes every book's planned move without writing anything -
// comic-server-3bz.4. Books that fail the profile's exclude rules are
// still included in the result (Skipped=true) so a caller can show why a
// book isn't part of the run, not just omit it silently.
func Plan(books []*library.ComicBook, opts PlanOptions) []PlannedMove {
	resolvePath := opts.ResolvePath
	if resolvePath == nil {
		resolvePath = func(p string) string { return p }
	}

	moves := make([]PlannedMove, 0, len(books))
	seenDestinations := make(map[string]int) // resolved path -> index into moves, for within-run collision detection

	for _, book := range books {
		move := PlannedMove{
			BookID:          book.ID,
			Series:          book.Series,
			Number:          book.Number,
			Title:           book.Title,
			OldRawPath:      book.FilePath,
			OldResolvedPath: resolvePath(book.FilePath),
		}

		shouldMove, ok := ShouldMove(book, opts.Exclude)
		if !ok {
			move.Failed = true
			move.FailReason = "exclude rule references an unsupported field"
			moves = append(moves, move)
			continue
		}
		if !shouldMove {
			move.Skipped = true
			moves = append(moves, move)
			continue
		}

		folder, folderOK := MakeFolderPath(book, opts.FolderTemplate, opts.Profile)
		fileName, fileOK := MakeFileName(book, opts.FileTemplate, opts.Profile)
		if !folderOK || !fileOK {
			move.Failed = true
			move.FailReason = "template references an unsupported or unresolvable field"
			moves = append(moves, move)
			continue
		}

		fullFileName := fileName + fileExtension(book, opts.Profile)
		move.NewFolder = folder
		move.NewFile = fullFileName
		move.NewRawPath = joinRawPath(append(append([]string{opts.BaseFolder}, folder...), fullFileName))
		move.NewResolvedPath = resolvePath(move.NewRawPath)

		// A book already at its own planned destination is a no-op, not
		// a collision, even if something (itself) is already there.
		if move.NewResolvedPath != move.OldResolvedPath {
			if opts.FileExists != nil && opts.FileExists(move.NewResolvedPath) {
				move.Collision = true
				move.CollisionReason = "a file already exists at the planned destination"
			} else if prior, exists := seenDestinations[move.NewResolvedPath]; exists {
				move.Collision = true
				move.CollisionReason = "another book in this run plans to the same destination"
				moves[prior].Collision = true
				moves[prior].CollisionReason = move.CollisionReason
			}
		}
		seenDestinations[move.NewResolvedPath] = len(moves)

		moves = append(moves, move)
	}

	return moves
}

// fileExtension mirrors PathMaker.make_file_name's own extension logic:
// the source file's real extension when there is one, or
// profile.FilelessFormat for a fileless book.
func fileExtension(book *library.ComicBook, profile Profile) string {
	if book.FilePath != "" {
		return extOf(book.FilePath)
	}
	return profile.FilelessFormat
}

// joinRawPath joins segments (BaseFolder + computed folder segments +
// filename) into one raw path - NOT filepath.Join, which would use the
// HOST's separator regardless of what style these segments actually are.
// BaseFolder is normally still Windows-style, exactly as recorded in
// losettingsx.dat (e.g. "G:\Comics") - unresolved until ResolvePath
// translates it later, same separation cbzconvert.Convert's own
// resolvePath parameter already established.
//
// BUT: `library-organizer import` now translates BaseFolder through
// server.library_source_root/library_mount_root at import time (a real
// user's Docker deployment has no "G:\Comics" to resolve against
// per-request), so BaseFolder can already be a POSIX-style absolute path
// by the time Plan runs - joining that with backslashes would produce a
// mixed-separator path nothing can open. Detect which style BaseFolder
// itself uses and join with that separator throughout, rather than
// assuming Windows unconditionally.
func joinRawPath(segments []string) string {
	sep := `\`
	if len(segments) > 0 && strings.HasPrefix(segments[0], "/") {
		sep = "/"
	}

	cleaned := make([]string, 0, len(segments))
	for _, s := range segments {
		s = strings.Trim(s, sep)
		if s != "" {
			cleaned = append(cleaned, s)
		}
	}
	joined := strings.Join(cleaned, sep)
	if sep == "/" {
		joined = "/" + joined
	}
	return joined
}
