package scaninfo

import (
	_ "embed"
	"strings"
)

// The default lists are the scanners.txt and blacklist.txt that ship with
// ComicRack's ScanInformationFromFilename plugin (tracker #70), so a new
// install detects the same tags the user's ComicRack does. Blacklist
// entries are regex fragments, not plain words.
var (
	//go:embed defaults/scanners.txt
	defaultScannersTxt string
	//go:embed defaults/blacklist.txt
	defaultBlacklistTxt string
)

// DefaultScanners returns a fresh copy of the built-in scanner names.
func DefaultScanners() []string { return splitNonEmptyLines(defaultScannersTxt) }

// DefaultBlacklist returns a fresh copy of the built-in blacklist patterns.
func DefaultBlacklist() []string { return splitNonEmptyLines(defaultBlacklistTxt) }

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
