// Package pathmap translates a comic library's raw recorded file paths
// (ComicBook.FilePath, in whatever separator style the ComicRack host that
// wrote the library XML used) into paths actually readable by whatever
// process needs to open the file - comic-server's own cover extraction and
// device-sync file transfer, and separately Komga's own filesystem view for
// its API-based matching. These are frequently different roots (e.g. a
// library authored on Windows, read by a Linux comic-server container that
// bind-mounts the real files at a different path than Komga's own
// container does) - see comic-server-64l/comic-server-ivq for the
// motivating history.
package pathmap

import (
	"fmt"
	"strings"
)

// TranslatePath converts localPath (rooted at localRoot) into the
// equivalent path rooted at remoteRoot, by swapping the prefix - the same
// approach as the *Arr apps' Remote Path Mapping. Directory structure below
// the root is assumed identical between the two.
//
// The localRoot prefix match is case-insensitive (Windows paths typically
// are), but everything after the root is preserved verbatim, since the
// target filesystem is usually Linux and case-sensitive.
func TranslatePath(localRoot, remoteRoot, localPath string) (string, error) {
	normPath := normalizeSlashes(localPath)
	normRoot := strings.TrimSuffix(normalizeSlashes(localRoot), "/")

	if len(normPath) < len(normRoot) || !strings.EqualFold(normPath[:len(normRoot)], normRoot) {
		return "", fmt.Errorf("path %q is not rooted at local_root %q", localPath, localRoot)
	}

	suffix := strings.TrimPrefix(normPath[len(normRoot):], "/")
	remote := strings.TrimSuffix(normalizeSlashes(remoteRoot), "/")
	if suffix == "" {
		return remote, nil
	}
	return remote + "/" + suffix, nil
}

// Resolve applies TranslatePath when both roots are configured, returning
// the translated path and true. Returns ("", false) - never an error, never
// rawPath - when either root is empty or the path isn't rooted at
// localRoot, so callers can fall through to a further fallback (e.g.
// another root pair) or use the original raw path as a last resort. This
// is the common "best effort, never fail outright" shape every consumer
// reading a library file path directly wants.
func Resolve(localRoot, remoteRoot, rawPath string) (string, bool) {
	if localRoot == "" || remoteRoot == "" {
		return "", false
	}
	translated, err := TranslatePath(localRoot, remoteRoot, rawPath)
	if err != nil {
		return "", false
	}
	return translated, true
}

func normalizeSlashes(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
