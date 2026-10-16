package comicvine

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// CVRef is a ComicVine issue or volume the user pasted (comic-server-5wer).
type CVRef struct {
	Kind string // "issue" or "volume"
	ID   int
}

var (
	reCVTypedID = regexp.MustCompile(`(?:^|[^0-9])(4\d{3})-(\d+)(?:[^0-9]|$)`)
	reBareID    = regexp.MustCompile(`^\d+$`)
)

// ComicVine type prefixes seen in site URLs and API ids.
var cvTypeNames = map[string]string{
	"4000": "issue", "4050": "volume",
	"4005": "character", "4010": "origin", "4015": "object", "4020": "location",
	"4040": "person", "4045": "story arc", "4055": "publisher", "4060": "team",
	"4065": "concept", "4070": "power",
}

// ParseCVRef reads a ComicVine issue or volume reference: a site URL such as
// https://comicvine.gamespot.com/some-name/4000-123456/, a typed id such as
// "4000-123456" or "4050-789", or a bare number (an issue id). Only the id is
// taken from the text; nothing is fetched from a pasted address.
func ParseCVRef(text string) (CVRef, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return CVRef{}, fmt.Errorf("paste a ComicVine issue link or id")
	}
	if strings.Contains(text, "://") {
		u, err := url.Parse(text)
		if err != nil || u.Host == "" {
			return CVRef{}, fmt.Errorf("that is not a valid link")
		}
		h := strings.ToLower(u.Hostname())
		if h != "comicvine.gamespot.com" && h != "comicvine.com" && !strings.HasSuffix(h, ".comicvine.com") {
			return CVRef{}, fmt.Errorf("that link is not from ComicVine")
		}
		text = u.Path
	}
	if reBareID.MatchString(text) {
		id, err := strconv.Atoi(text)
		if err != nil || id <= 0 {
			return CVRef{}, fmt.Errorf("that id is not valid")
		}
		return CVRef{Kind: "issue", ID: id}, nil
	}
	m := reCVTypedID.FindStringSubmatch(text)
	if m == nil {
		return CVRef{}, fmt.Errorf("no ComicVine issue or volume id found (it looks like 4000-123456)")
	}
	kind, ok := cvTypeNames[m[1]]
	if !ok {
		return CVRef{}, fmt.Errorf("unknown ComicVine id type %s", m[1])
	}
	if kind != "issue" && kind != "volume" {
		return CVRef{}, fmt.Errorf("that link is a %s, not an issue or volume", kind)
	}
	id, err := strconv.Atoi(m[2])
	if err != nil || id <= 0 {
		return CVRef{}, fmt.Errorf("that id is not valid")
	}
	return CVRef{Kind: kind, ID: id}, nil
}
