// Package watchfolder scans configured "dump" directories - staging
// folders comic files land in before they're added to the ComicRack
// library at all - for files not yet backed by a library book record.
// This is the comic-server-native replacement for a ComicRack "0 Day
// Folder" smart list, which only ever showed files that were BOTH in a
// watched path AND already present in ComicDb.xml - see comic-server-chh.
package watchfolder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/duckpuppy/comic-server/internal/comicvine"
)

// comicExtensions are the archive formats ComicRack (and comic-server)
// treats as a comic book file - mirrors the set comicvine.ParseFilename
// strips, plus the raw archive extensions ComicRack accepts directly
// (before any "needs conversion" processing - see workflow.needsConvertToCBZ).
var comicExtensions = map[string]bool{
	".cbz": true, ".cbr": true, ".cb7": true, ".cbt": true,
	".zip": true, ".rar": true, ".7z": true,
}

// IsComicFile reports whether path's extension is a recognized comic
// archive format, case-insensitively.
func IsComicFile(path string) bool {
	return comicExtensions[strings.ToLower(filepath.Ext(path))]
}

// ambiguousExtensions are archive formats that are just as often not a comic.
// The "cb*" extensions say "comic" by name; these do not.
var ambiguousExtensions = map[string]bool{".zip": true, ".rar": true, ".7z": true}

func isAmbiguousArchive(path string) bool {
	return ambiguousExtensions[strings.ToLower(filepath.Ext(path))]
}

// listPages returns the image page names of an archive without reading any
// page data. A variable so tests can replace it.
var listPages = comicvine.ListPageNames

// pageCheck remembers the answer per file (path, size, modified time) so a
// refresh of the New Files list does not open the same archives again.
var pageCheck sync.Map // string -> bool

// hasImagePages reports whether an ambiguous archive holds at least one image.
// Reads only the archive's file list. A file that cannot be opened counts as
// "no": it would fail the same way on import.
func hasImagePages(path string, info os.FileInfo) bool {
	key := path
	if info != nil {
		key = fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	}
	if v, ok := pageCheck.Load(key); ok {
		return v.(bool)
	}
	names, err := listPages(path)
	ok := err == nil && len(names) > 0
	pageCheck.Store(key, ok)
	return ok
}

// DiscoveredFile is one comic archive found under a watch folder that
// isn't yet backed by any library book's FilePath.
type DiscoveredFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"` // RFC3339, empty if unavailable
}

// Scan walks each folder recursively for comic archive files, returning
// every one whose path is not a key in knownPaths (already-imported book
// FilePaths, normalized the same way as the discovered paths - see
// normalizePath). A folder that doesn't exist or can't be read is skipped
// rather than failing the whole scan - one misconfigured/removed watch
// folder shouldn't hide files sitting in the others. Results are sorted by
// path for a stable, deterministic order across calls.
func Scan(folders []string, knownPaths map[string]struct{}) ([]DiscoveredFile, error) {
	var found []DiscoveredFile
	for _, folder := range folders {
		if folder == "" {
			continue
		}
		_ = filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				// Unreadable file/dir - skip it, don't abort the whole
				// walk (a permission-denied subfolder shouldn't hide
				// files elsewhere in the same watch folder).
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !IsComicFile(path) {
				return nil
			}
			if _, known := knownPaths[normalizePath(path)]; known {
				return nil
			}
			info, err := d.Info()
			// A .zip, .rar or .7z may be a manual archive or other data, not a
			// comic. Only those three are opened, and only for the file list.
			if isAmbiguousArchive(path) && !hasImagePages(path, info) {
				return nil
			}
			var size int64
			var modTime string
			if err == nil {
				size = info.Size()
				modTime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			found = append(found, DiscoveredFile{Path: path, Size: size, ModTime: modTime})
			return nil
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Path < found[j].Path })
	return found, nil
}

// normalizePath makes a filesystem path comparable against a library
// book's FilePath regardless of separator style - mirrors
// pathmap.normalizeSlashes rather than importing internal/pathmap for one
// function, since this package has no other need for it.
func normalizePath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// KnownPathSet builds the normalized-path lookup Scan needs from every
// book's current FilePath - callers pass the result of
// library.Backend.GetAllBooks().
func KnownPathSet(filePaths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(filePaths))
	for _, p := range filePaths {
		if p == "" {
			continue
		}
		set[normalizePath(p)] = struct{}{}
	}
	return set
}
