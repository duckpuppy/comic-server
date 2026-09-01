package comicvine

import (
	"fmt"
	"html"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ScraperConfig controls which ComicBook fields the metadata writer updates
// and how conflicts with existing data are resolved. All Update* fields
// default to true (see DefaultScraperConfig).
type ScraperConfig struct {
	// Per-field toggles
	UpdateSeries      bool
	UpdateNumber      bool
	UpdateTitle       bool
	UpdateSummary     bool
	UpdatePublisher   bool
	UpdateImprint     bool
	UpdateVolume      bool
	UpdatePublished   bool // Year/Month/Day from cover_date
	UpdateReleased    bool // ReleasedTime from store_date
	UpdateWriter      bool
	UpdatePenciller   bool
	UpdateInker       bool
	UpdateColorist    bool
	UpdateLetterer    bool
	UpdateCoverArtist bool
	UpdateEditor      bool
	UpdateCharacters  bool
	UpdateTeams       bool
	UpdateLocations   bool
	UpdateStoryArcs   bool
	UpdateWebpage     bool

	// Behavior
	OverwriteExisting bool // true = overwrite, false = only fill empty fields
	IgnoreBlanks      bool // true = don't overwrite existing data with blank CV data
	ConvertImprints   bool // true = move imprints to Imprint field, use parent as Publisher
	NoteScrapeDate    bool // true = add scrape timestamp to Notes

	// Publisher mappings
	PublisherAliases map[string]string // e.g. "Marvel Italia" -> "Marvel Comics"
	ImprintMappings  map[string]string // e.g. "Vertigo" -> "DC Comics"
}

// DefaultScraperConfig returns a ScraperConfig with all field updates enabled
// and the standard comic-vine-scraper imprint mappings.
func DefaultScraperConfig() ScraperConfig {
	return ScraperConfig{
		UpdateSeries:      true,
		UpdateNumber:      true,
		UpdateTitle:       true,
		UpdateSummary:     true,
		UpdatePublisher:   true,
		UpdateImprint:     true,
		UpdateVolume:      true,
		UpdatePublished:   true,
		UpdateReleased:    true,
		UpdateWriter:      true,
		UpdatePenciller:   true,
		UpdateInker:       true,
		UpdateColorist:    true,
		UpdateLetterer:    true,
		UpdateCoverArtist: true,
		UpdateEditor:      true,
		UpdateCharacters:  true,
		UpdateTeams:       true,
		UpdateLocations:   true,
		UpdateStoryArcs:   true,
		UpdateWebpage:     true,

		OverwriteExisting: true,
		IgnoreBlanks:      true,
		ConvertImprints:   true,
		NoteScrapeDate:    false,

		PublisherAliases: map[string]string{},
		ImprintMappings:  copyImprintMap(),
	}
}

// defaultImprintMap maps known ComicVine imprint publisher names to their
// parent publisher, ported from comic-vine-scraper's cvimprints.py.
var defaultImprintMap = map[string]string{
	"2000AD":                          "DC Comics",
	"Adventure":                       "Malibu",
	"Aircel Publishing":               "Malibu",
	"America's Best Comics":           "DC Comics",
	"Amerotica":                       "Nbm",
	"Antimatter":                      "Amryl Entertainment",
	"Apparat":                         "Avatar Press",
	"Archaia":                         "Boom!",
	"Berger Books":                    "Dark Horse Comics",
	"BOOM! Box":                       "Boom!",
	"Boundless Comics":                "Avatar Press",
	"Black Bull":                      "Wizard",
	"Black Crown":                     "IDW Publishing",
	"Blu Manga":                       "Tokyopop",
	"CMX":                             "DC Comics",
	"Chaos! Comics":                   "Dynamite Entertainment",
	"Cliffhanger":                     "DC Comics",
	"Comic Bom Bom":                   "Kodansha",
	"ComicsLit":                       "Nbm",
	"Curtis Magazines":                "Marvel",
	"Danger Zone":                     "Action Lab",
	"Dark Horse Books":                "Dark Horse Comics",
	"Dark Horse Manga":                "Dark Horse Comics",
	"Desperado Publishing":            "Image",
	"Epic":                            "Marvel",
	"Eternity":                        "Malibu",
	"Eurotica":                        "Nbm",
	"Focus":                           "DC Comics",
	"Helix":                           "DC Comics",
	"Hero Comics":                     "Heroic Publishing",
	"Homage comics":                   "DC Comics",
	"Hudson Street Press":             "Penguin Group",
	"Icon Comics":                     "Marvel",
	"Impact":                          "DC Comics",
	"Jets Comics":                     "Hakusensha",
	"KaBOOM!":                         "Boom!",
	"KiZoic":                          "Ape Entertainment",
	"Kodansha Comics Digital-First!":  "Kodansha",
	"Kodansha Comics USA":             "Kodansha",
	"MAD":                             "DC Comics",
	"Marvel Digital Comics Unlimited": "Marvel",
	"Marvel Knights":                  "Marvel",
	"Marvel Music":                    "Marvel",
	"Marvel Soleil":                   "Marvel",
	"Marvel UK":                       "Marvel",
	"Maverick":                        "Dark Horse Comics",
	"Max":                             "Marvel",
	"Milestone":                       "DC Comics",
	"Minx":                            "DC Comics",
	"Papercutz":                       "Nbm",
	"Paradox Press":                   "DC Comics",
	"Piranha Press":                   "DC Comics",
	"Quillion":                        "Lion Forge Comics",
	"Razorline":                       "Marvel",
	"Roar Comics":                     "Lion Forge Comics",
	"ShadowLine":                      "Image",
	"Silverline":                      "Image",
	"Sin Factory Comix":               "Radio Comix",
	"Skybound":                        "Image",
	"Slave Labor":                     "Slg Publishing",
	"Star Comics":                     "Marvel",
	"Tangent Comics":                  "DC Comics",
	"Titan Books":                     "Titan Comics",
	"Todd McFarlane Productions":      "Image",
	"Tokuma Comics":                   "Tokuma Shoten",
	"Top Cow":                         "Image",
	"Top Shelf":                       "IDW Publishing",
	"Ultraverse":                      "Malibu",
	"Vertical":                        "Kodansha",
	"Vertigo":                         "DC Comics",
	"Wildstorm":                       "DC Comics",
	"Zuda Comics":                     "DC Comics",
}

func copyImprintMap() map[string]string {
	m := make(map[string]string, len(defaultImprintMap))
	maps.Copy(m, defaultImprintMap)
	return m
}

// WriteResult reports what ApplyMetadata (or WriteMetadata) changed.
type WriteResult struct {
	Changed bool
	Fields  []string
}

// WriteMetadata maps ComicVine volume/issue data onto book and, if anything
// changed, persists it via the backend.
func WriteMetadata(backend library.Backend, book *library.ComicBook, volume Volume, detail *IssueDetail, cfg ScraperConfig) (WriteResult, error) {
	result := ApplyMetadata(book, volume, detail, cfg)
	if !result.Changed {
		return result, nil
	}
	if err := backend.UpdateBook(book); err != nil {
		return result, fmt.Errorf("update book: %w", err)
	}
	backend.MarkDirty(book.ID)
	return result, nil
}

// ApplyMetadata maps ComicVine volume/issue data onto book's fields in
// place, honoring cfg's per-field toggles and overwrite/blank-handling
// behavior. It returns which fields were actually changed.
func ApplyMetadata(book *library.ComicBook, volume Volume, detail *IssueDetail, cfg ScraperConfig) WriteResult {
	var result WriteResult
	note := func(name string, did bool) {
		if did {
			result.Changed = true
			result.Fields = append(result.Fields, name)
		}
	}

	publisher, imprint := resolvePublisher(volume.Publisher.Name, cfg)

	note("Series", setString(&book.Series, volume.Name, cfg.UpdateSeries, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Number", setString(&book.Number, detail.IssueNumber, cfg.UpdateNumber, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Title", setString(&book.Title, detail.Name, cfg.UpdateTitle, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Publisher", setString(&book.Publisher, publisher, cfg.UpdatePublisher, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	if cfg.ConvertImprints {
		note("Imprint", setString(&book.Imprint, imprint, cfg.UpdateImprint, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	}
	note("Volume", setInt(&book.Volume, parseVolumeYear(volume.StartYear), cfg.UpdateVolume, cfg.OverwriteExisting, cfg.IgnoreBlanks))

	if cfg.UpdatePublished {
		year, month, day := parseCoverDate(detail.CoverDate)
		note("Year", setInt(&book.Year, year, true, cfg.OverwriteExisting, cfg.IgnoreBlanks))
		note("Month", setInt(&book.Month, month, true, cfg.OverwriteExisting, cfg.IgnoreBlanks))
		note("Day", setInt(&book.Day, day, true, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	}
	if cfg.UpdateReleased {
		note("ReleasedTime", setReleasedTime(book, detail.StoreDate, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	}

	note("Summary", setString(&book.Summary, StripHTML(detail.Description), cfg.UpdateSummary, cfg.OverwriteExisting, cfg.IgnoreBlanks))

	writers, pencillers, inkers, colorists, letterers, coverArtists, editors := splitCredits(detail.PersonCredits)
	note("Writer", setString(&book.Writer, writers, cfg.UpdateWriter, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Penciller", setString(&book.Penciller, pencillers, cfg.UpdatePenciller, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Inker", setString(&book.Inker, inkers, cfg.UpdateInker, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Colorist", setString(&book.Colorist, colorists, cfg.UpdateColorist, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Letterer", setString(&book.Letterer, letterers, cfg.UpdateLetterer, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("CoverArtist", setString(&book.CoverArtist, coverArtists, cfg.UpdateCoverArtist, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Editor", setString(&book.Editor, editors, cfg.UpdateEditor, cfg.OverwriteExisting, cfg.IgnoreBlanks))

	note("Characters", setString(&book.Characters, joinNamedCredits(detail.CharacterCredits), cfg.UpdateCharacters, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Teams", setString(&book.Teams, joinNamedCredits(detail.TeamCredits), cfg.UpdateTeams, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("Locations", setString(&book.Locations, joinNamedCredits(detail.LocationCredits), cfg.UpdateLocations, cfg.OverwriteExisting, cfg.IgnoreBlanks))
	note("StoryArc", setString(&book.StoryArc, joinNamedCredits(detail.StoryArcCredits), cfg.UpdateStoryArcs, cfg.OverwriteExisting, cfg.IgnoreBlanks))

	note("Web", setString(&book.Web, detail.SiteDetailURL, cfg.UpdateWebpage, cfg.OverwriteExisting, cfg.IgnoreBlanks))

	// Identity tracking is always applied, independent of the per-field toggles above.
	newStore := library.SetCustomValue(book.CustomValuesStore, "comicvine_volume", strconv.Itoa(volume.ID))
	newStore = library.SetCustomValue(newStore, "comicvine_issue", strconv.Itoa(detail.ID))
	if newStore != book.CustomValuesStore {
		book.CustomValuesStore = newStore
		note("CustomValuesStore", true)
	}

	newTags := setCVDBTag(book.Tags, detail.ID)
	if newTags != book.Tags {
		book.Tags = newTags
		note("Tags", true)
	}

	if cfg.NoteScrapeDate {
		note("Notes", appendScrapeNote(book, time.Now()))
	}

	return result
}

func resolvePublisher(cvPublisher string, cfg ScraperConfig) (publisher, imprint string) {
	name := cvPublisher
	if alias, ok := cfg.PublisherAliases[name]; ok {
		name = alias
	}
	if cfg.ConvertImprints {
		if parent, ok := cfg.ImprintMappings[name]; ok {
			return parent, name
		}
	}
	return name, ""
}

// setString updates *dst to value, subject to the enabled/overwrite/ignoreBlanks
// rules, and reports whether it made a change.
func setString(dst *string, value string, enabled, overwrite, ignoreBlanks bool) bool {
	if !enabled {
		return false
	}
	if value == "" && ignoreBlanks {
		return false
	}
	if *dst != "" && !overwrite {
		return false
	}
	if *dst == value {
		return false
	}
	*dst = value
	return true
}

func setInt(dst *int, value int, enabled, overwrite, ignoreBlanks bool) bool {
	if !enabled {
		return false
	}
	if value == 0 && ignoreBlanks {
		return false
	}
	if *dst != 0 && !overwrite {
		return false
	}
	if *dst == value {
		return false
	}
	*dst = value
	return true
}

func setReleasedTime(book *library.ComicBook, storeDate string, overwrite, ignoreBlanks bool) bool {
	t, ok := parseDate(storeDate)
	if !ok {
		if ignoreBlanks {
			return false
		}
		if book.ReleasedTime.IsZero() {
			return false
		}
		if !overwrite {
			return false
		}
		book.ReleasedTime = library.ComicTime{}
		return true
	}
	if !book.ReleasedTime.IsZero() && !overwrite {
		return false
	}
	if book.ReleasedTime.Time.Equal(t) {
		return false
	}
	book.ReleasedTime = library.ComicTime{Time: t}
	return true
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseCoverDate(s string) (year, month, day int) {
	t, ok := parseDate(s)
	if !ok {
		return 0, 0, 0
	}
	return t.Year(), int(t.Month()), t.Day()
}

// splitCredits buckets person_credits by role into comma-separated name lists.
// ComicVine encodes multiple roles per person as a comma-separated role string.
func splitCredits(credits []PersonCredit) (writers, pencillers, inkers, colorists, letterers, coverArtists, editors string) {
	var w, p, i, c, l, cov, e []string
	for _, credit := range credits {
		for role := range strings.SplitSeq(credit.Role, ",") {
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "writer":
				w = appendUnique(w, credit.Name)
			case "penciller", "penciler":
				p = appendUnique(p, credit.Name)
			case "inker":
				i = appendUnique(i, credit.Name)
			case "artist":
				// ComicVine's generic "artist" role covers both penciling and
				// inking when the two aren't credited separately.
				p = appendUnique(p, credit.Name)
				i = appendUnique(i, credit.Name)
			case "colorist", "colourist", "colorer":
				c = appendUnique(c, credit.Name)
			case "letterer":
				l = appendUnique(l, credit.Name)
			case "cover", "cover artist":
				cov = appendUnique(cov, credit.Name)
			case "editor":
				e = appendUnique(e, credit.Name)
			}
		}
	}
	return strings.Join(w, ", "), strings.Join(p, ", "), strings.Join(i, ", "),
		strings.Join(c, ", "), strings.Join(l, ", "), strings.Join(cov, ", "), strings.Join(e, ", ")
}

func appendUnique(list []string, name string) []string {
	if slices.Contains(list, name) {
		return list
	}
	return append(list, name)
}

func joinNamedCredits(credits []NamedCredit) string {
	if len(credits) == 0 {
		return ""
	}
	names := make([]string, len(credits))
	for i, c := range credits {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

func appendScrapeNote(book *library.ComicBook, at time.Time) bool {
	stamp := "Scraped " + at.Format("2006-01-02")
	if strings.Contains(book.Notes, stamp) {
		return false
	}
	if book.Notes == "" {
		book.Notes = stamp
	} else {
		book.Notes = book.Notes + "; " + stamp
	}
	return true
}

// setCVDBTag replaces any existing "CVDB<id>" tag with the current issue's
// tag, so re-scraping doesn't accumulate stale identifiers.
func setCVDBTag(tags string, issueID int) string {
	newTag := fmt.Sprintf("CVDB%d", issueID)
	var kept []string
	for t := range strings.SplitSeq(tags, ",") {
		t = strings.TrimSpace(t)
		if t == "" || strings.HasPrefix(t, "CVDB") {
			continue
		}
		kept = append(kept, t)
	}
	kept = append(kept, newTag)
	return strings.Join(kept, ", ")
}

var (
	reBR      = regexp.MustCompile(`(?i)<br\s*/?>`)
	reCloseP  = regexp.MustCompile(`(?i)</p>`)
	reAnyTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	reMultiNL = regexp.MustCompile(`\n{3,}`)
)

// StripHTML converts ComicVine's HTML description field into plain text:
// <br> and </p> become newlines, all other tags are removed, and HTML
// entities are decoded.
func StripHTML(s string) string {
	if s == "" {
		return ""
	}
	s = reBR.ReplaceAllString(s, "\n")
	s = reCloseP.ReplaceAllString(s, "\n\n")
	s = reAnyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reMultiNL.ReplaceAllString(s, "\n\n")

	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
