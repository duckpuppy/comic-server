package comicvine

import (
	"errors"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

var errTest = errors.New("update failed")

// fakeBackend is a minimal library.Backend implementation for testing
// WriteMetadata's persistence calls.
type fakeBackend struct {
	updated   []*library.ComicBook
	dirty     []string
	updateErr error
	books     map[string]*library.ComicBook
}

func (f *fakeBackend) GetBook(id string) (*library.ComicBook, error) {
	if f.books == nil {
		return nil, nil
	}
	return f.books[id], nil
}
func (f *fakeBackend) GetAllBooks() ([]library.ComicBook, error)              { return nil, nil }
func (f *fakeBackend) FindListByID(id string) (*library.ComicListItem, error) { return nil, nil }
func (f *fakeBackend) FindList(name string) (*library.ComicListItem, error)   { return nil, nil }
func (f *fakeBackend) GetAllLists() ([]library.ComicListItem, error)          { return nil, nil }
func (f *fakeBackend) CreateList(list *library.ComicListItem) error           { return nil }
func (f *fakeBackend) UpdateList(list *library.ComicListItem) error           { return nil }
func (f *fakeBackend) DeleteList(id string) error                             { return nil }
func (f *fakeBackend) MoveList(id, parentID string) error                     { return nil }
func (f *fakeBackend) MatchBooks(list *library.ComicListItem) ([]*library.ComicBook, error) {
	return nil, nil
}
func (f *fakeBackend) GetBooksForList(list *library.ComicListItem) ([]*library.ComicBook, error) {
	return nil, nil
}
func (f *fakeBackend) UpdateBook(book *library.ComicBook) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, book)
	return nil
}
func (f *fakeBackend) UpdateBooks(books []*library.ComicBook) error { return nil }
func (f *fakeBackend) MarkDirty(bookID string)                      { f.dirty = append(f.dirty, bookID) }
func (f *fakeBackend) MarkManyDirty(bookIDs []string)               {}
func (f *fakeBackend) Flush() error                                 { return nil }
func (f *fakeBackend) Close() error                                 { return nil }
func (f *fakeBackend) LibraryID() string                            { return "" }
func (f *fakeBackend) LibraryName() string                          { return "" }
func (f *fakeBackend) BookCount() int                               { return 0 }
func (f *fakeBackend) CanPersist() bool                             { return true }

func testVolume() Volume {
	return Volume{
		ID:            100,
		Name:          "Batman",
		StartYear:     "2016",
		Publisher:     Publisher{Name: "Vertigo"},
		CountOfIssues: 85,
	}
}

func testIssueDetail() *IssueDetail {
	d := &IssueDetail{
		ID:            1001,
		IssueNumber:   "1",
		Name:          "I Am Gotham",
		CoverDate:     "2016-08-01",
		StoreDate:     "2016-06-15",
		Description:   "<p>The <strong>Dark Knight</strong> returns.<br>Again.</p>",
		SiteDetailURL: "https://comicvine.gamespot.com/batman-1/4000-1001/",
		PersonCredits: []PersonCredit{
			{Name: "Tom King", Role: "writer"},
			{Name: "David Finch", Role: "penciler, cover"},
			{Name: "Danny Miki", Role: "inker"},
			{Name: "Jordie Bellaire", Role: "colorist"},
			{Name: "Deron Bennett", Role: "letterer"},
			{Name: "Mark Doyle", Role: "editor"},
		},
		CharacterCredits: []NamedCredit{{Name: "Batman"}, {Name: "Gotham (Character)"}},
		TeamCredits:      []NamedCredit{{Name: "Justice League"}},
		LocationCredits:  []NamedCredit{{Name: "Gotham City"}},
		StoryArcCredits:  []NamedCredit{{Name: "I Am Gotham"}},
	}
	d.Volume.ID = 100
	d.Volume.Name = "Batman"
	return d
}

func TestApplyMetadata_FieldMapping(t *testing.T) {
	book := &library.ComicBook{ID: "book-1"}
	cfg := DefaultScraperConfig()

	result := ApplyMetadata(book, testVolume(), testIssueDetail(), cfg)
	if !result.Changed {
		t.Fatal("expected changes")
	}

	if book.Series != "Batman" {
		t.Errorf("Series = %q", book.Series)
	}
	if book.Number != "1" {
		t.Errorf("Number = %q", book.Number)
	}
	if book.Title != "I Am Gotham" {
		t.Errorf("Title = %q", book.Title)
	}
	if book.Publisher != "DC Comics" || book.Imprint != "Vertigo" {
		t.Errorf("Publisher/Imprint = %q/%q, want DC Comics/Vertigo", book.Publisher, book.Imprint)
	}
	if book.Volume != 2016 {
		t.Errorf("Volume = %d, want 2016", book.Volume)
	}
	if book.Year != 2016 || book.Month != 8 || book.Day != 1 {
		t.Errorf("Year/Month/Day = %d/%d/%d", book.Year, book.Month, book.Day)
	}
	if book.ReleasedTime.IsZero() || book.ReleasedTime.Format("2006-01-02") != "2016-06-15" {
		t.Errorf("ReleasedTime = %v", book.ReleasedTime)
	}
	if book.Summary != "The Dark Knight returns.\nAgain." {
		t.Errorf("Summary = %q", book.Summary)
	}
	if book.Writer != "Tom King" {
		t.Errorf("Writer = %q", book.Writer)
	}
	if book.Penciller != "David Finch" {
		t.Errorf("Penciller = %q", book.Penciller)
	}
	if book.Inker != "Danny Miki" {
		t.Errorf("Inker = %q", book.Inker)
	}
	if book.Colorist != "Jordie Bellaire" {
		t.Errorf("Colorist = %q", book.Colorist)
	}
	if book.Letterer != "Deron Bennett" {
		t.Errorf("Letterer = %q", book.Letterer)
	}
	if book.CoverArtist != "David Finch" {
		t.Errorf("CoverArtist = %q", book.CoverArtist)
	}
	if book.Editor != "Mark Doyle" {
		t.Errorf("Editor = %q", book.Editor)
	}
	if book.Characters != "Batman, Gotham (Character)" {
		t.Errorf("Characters = %q", book.Characters)
	}
	if book.Teams != "Justice League" {
		t.Errorf("Teams = %q", book.Teams)
	}
	if book.Locations != "Gotham City" {
		t.Errorf("Locations = %q", book.Locations)
	}
	if book.StoryArc != "I Am Gotham" {
		t.Errorf("StoryArc = %q", book.StoryArc)
	}
	if book.Web != "https://comicvine.gamespot.com/batman-1/4000-1001/" {
		t.Errorf("Web = %q", book.Web)
	}
	if book.CustomValuesStore != ",comicvine_volume=100,comicvine_issue=1001" {
		t.Errorf("CustomValuesStore = %q", book.CustomValuesStore)
	}
	if book.Tags != "CVDB1001" {
		t.Errorf("Tags = %q", book.Tags)
	}
}

func TestApplyMetadata_HTMLStripVariants(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"<p>Hello world.</p>", "Hello world."},
		{"Line one<br>Line two", "Line one\nLine two"},
		{"Line one<br/>Line two", "Line one\nLine two"},
		{"<p>Para one.</p><p>Para two.</p>", "Para one.\n\nPara two."},
		{"<em>Emphasis</em> and <strong>bold</strong>", "Emphasis and bold"},
		{"<ul><li>One</li><li>Two</li></ul>", "OneTwo"},
		{"A &amp; B &lt;tag&gt; &quot;quoted&quot;", `A & B <tag> "quoted"`},
		{"<a href=\"http://x\">link</a>", "link"},
	}
	for _, tt := range tests {
		if got := StripHTML(tt.in); got != tt.want {
			t.Errorf("StripHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestApplyMetadata_ImprintConversion(t *testing.T) {
	cfg := DefaultScraperConfig()

	book := &library.ComicBook{}
	v := Volume{Name: "Ultimate Spider-Man", Publisher: Publisher{Name: "Icon Comics"}}
	ApplyMetadata(book, v, &IssueDetail{}, cfg)
	if book.Publisher != "Marvel" || book.Imprint != "Icon Comics" {
		t.Errorf("Publisher/Imprint = %q/%q, want Marvel/Icon Comics", book.Publisher, book.Imprint)
	}

	book2 := &library.ComicBook{}
	v2 := Volume{Name: "Action Comics", Publisher: Publisher{Name: "DC Comics"}}
	ApplyMetadata(book2, v2, &IssueDetail{}, cfg)
	if book2.Publisher != "DC Comics" || book2.Imprint != "" {
		t.Errorf("Publisher/Imprint = %q/%q, want DC Comics/<empty> for non-imprint publisher", book2.Publisher, book2.Imprint)
	}

	// ConvertImprints=false: publisher is passed through unmapped, imprint untouched.
	cfg.ConvertImprints = false
	book3 := &library.ComicBook{}
	ApplyMetadata(book3, v, &IssueDetail{}, cfg)
	if book3.Publisher != "Icon Comics" || book3.Imprint != "" {
		t.Errorf("Publisher/Imprint = %q/%q, want Icon Comics/<empty> when ConvertImprints=false", book3.Publisher, book3.Imprint)
	}
}

func TestApplyMetadata_PublisherAliases(t *testing.T) {
	cfg := DefaultScraperConfig()
	cfg.PublisherAliases = map[string]string{"Marvel Italia": "Marvel"}

	book := &library.ComicBook{}
	v := Volume{Name: "X", Publisher: Publisher{Name: "Marvel Italia"}}
	ApplyMetadata(book, v, &IssueDetail{}, cfg)
	if book.Publisher != "Marvel" {
		t.Errorf("Publisher = %q, want Marvel (via alias)", book.Publisher)
	}
}

func TestApplyMetadata_OverwriteExistingFalse(t *testing.T) {
	book := &library.ComicBook{
		Series:    "Existing Series",
		Publisher: "Existing Publisher",
		Writer:    "Existing Writer",
	}
	cfg := DefaultScraperConfig()
	cfg.OverwriteExisting = false

	ApplyMetadata(book, testVolume(), testIssueDetail(), cfg)

	if book.Series != "Existing Series" {
		t.Errorf("Series = %q, should not have been overwritten", book.Series)
	}
	if book.Publisher != "Existing Publisher" {
		t.Errorf("Publisher = %q, should not have been overwritten", book.Publisher)
	}
	if book.Writer != "Existing Writer" {
		t.Errorf("Writer = %q, should not have been overwritten", book.Writer)
	}
	// Fields that were empty should still get filled in.
	if book.Title != "I Am Gotham" {
		t.Errorf("Title = %q, empty field should have been filled", book.Title)
	}
}

func TestApplyMetadata_IgnoreBlanksTrue(t *testing.T) {
	book := &library.ComicBook{Summary: "Existing summary", Writer: "Existing Writer"}
	cfg := DefaultScraperConfig()

	blankDetail := &IssueDetail{ID: 1, IssueNumber: "1"} // no description, no credits
	ApplyMetadata(book, testVolume(), blankDetail, cfg)

	if book.Summary != "Existing summary" {
		t.Errorf("Summary = %q, blank CV data should not have cleared it", book.Summary)
	}
	if book.Writer != "Existing Writer" {
		t.Errorf("Writer = %q, blank CV data should not have cleared it", book.Writer)
	}
}

func TestApplyMetadata_IgnoreBlanksFalse(t *testing.T) {
	book := &library.ComicBook{Summary: "Existing summary"}
	cfg := DefaultScraperConfig()
	cfg.IgnoreBlanks = false

	blankDetail := &IssueDetail{ID: 1, IssueNumber: "1"}
	ApplyMetadata(book, testVolume(), blankDetail, cfg)

	if book.Summary != "" {
		t.Errorf("Summary = %q, want cleared when IgnoreBlanks=false", book.Summary)
	}
}

func TestApplyMetadata_CustomValuesStorePreservesOtherKeys(t *testing.T) {
	book := &library.ComicBook{CustomValuesStore: ",some_other_key=abc,comicvine_volume=999"}
	cfg := DefaultScraperConfig()

	ApplyMetadata(book, testVolume(), testIssueDetail(), cfg)

	if book.CustomValuesStore != ",some_other_key=abc,comicvine_volume=100,comicvine_issue=1001" {
		t.Errorf("CustomValuesStore = %q", book.CustomValuesStore)
	}
}

func TestApplyMetadata_TagsReplacesOldCVDBTag(t *testing.T) {
	book := &library.ComicBook{Tags: "favorite, CVDB555, classic"}
	cfg := DefaultScraperConfig()

	ApplyMetadata(book, testVolume(), testIssueDetail(), cfg)

	if book.Tags != "favorite, classic, CVDB1001" {
		t.Errorf("Tags = %q", book.Tags)
	}
}

func TestApplyMetadata_PerFieldTogglesDisabled(t *testing.T) {
	book := &library.ComicBook{}
	cfg := DefaultScraperConfig()
	cfg.UpdateSeries = false
	cfg.UpdateWriter = false
	cfg.UpdateSummary = false

	ApplyMetadata(book, testVolume(), testIssueDetail(), cfg)

	if book.Series != "" {
		t.Errorf("Series = %q, want untouched (disabled)", book.Series)
	}
	if book.Writer != "" {
		t.Errorf("Writer = %q, want untouched (disabled)", book.Writer)
	}
	if book.Summary != "" {
		t.Errorf("Summary = %q, want untouched (disabled)", book.Summary)
	}
	// Other fields still get updated.
	if book.Title != "I Am Gotham" {
		t.Errorf("Title = %q, want updated", book.Title)
	}
}

func TestApplyMetadata_NoteScrapeDate(t *testing.T) {
	book := &library.ComicBook{}
	cfg := DefaultScraperConfig()
	cfg.NoteScrapeDate = true

	result := ApplyMetadata(book, testVolume(), testIssueDetail(), cfg)
	if book.Notes == "" {
		t.Error("expected scrape timestamp in Notes")
	}
	found := false
	for _, f := range result.Fields {
		if f == "Notes" {
			found = true
		}
	}
	if !found {
		t.Error("expected Notes in result.Fields")
	}
}

func TestWriteMetadata_PersistsOnChange(t *testing.T) {
	backend := &fakeBackend{}
	book := &library.ComicBook{ID: "book-1"}

	result, err := WriteMetadata(backend, book, testVolume(), testIssueDetail(), DefaultScraperConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("expected changes")
	}
	if len(backend.updated) != 1 || backend.updated[0] != book {
		t.Errorf("UpdateBook not called with book: %+v", backend.updated)
	}
	if len(backend.dirty) != 1 || backend.dirty[0] != "book-1" {
		t.Errorf("MarkDirty not called with book-1: %+v", backend.dirty)
	}
}

func TestWriteMetadata_NoChangeSkipsPersist(t *testing.T) {
	backend := &fakeBackend{}
	book := &library.ComicBook{
		ID:                "book-1",
		Series:            "Batman",
		Number:            "1",
		Title:             "I Am Gotham",
		Publisher:         "DC Comics",
		Imprint:           "Vertigo",
		Volume:            2016,
		Year:              2016,
		Month:             8,
		Day:               1,
		Summary:           "The Dark Knight returns.\nAgain.",
		Writer:            "Tom King",
		Penciller:         "David Finch",
		Inker:             "Danny Miki",
		Colorist:          "Jordie Bellaire",
		Letterer:          "Deron Bennett",
		CoverArtist:       "David Finch",
		Editor:            "Mark Doyle",
		Characters:        "Batman, Gotham (Character)",
		Teams:             "Justice League",
		Locations:         "Gotham City",
		StoryArc:          "I Am Gotham",
		Web:               "https://comicvine.gamespot.com/batman-1/4000-1001/",
		CustomValuesStore: ",comicvine_volume=100,comicvine_issue=1001",
		Tags:              "CVDB1001",
	}
	book.ReleasedTime = library.ComicTime{}
	// Set ReleasedTime to match so it isn't reported changed.
	t2, _ := parseDate("2016-06-15")
	book.ReleasedTime = library.ComicTime{Time: t2}

	result, err := WriteMetadata(backend, book, testVolume(), testIssueDetail(), DefaultScraperConfig())
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Errorf("expected no changes, got Fields=%v", result.Fields)
	}
	if len(backend.updated) != 0 {
		t.Errorf("UpdateBook should not have been called")
	}
	if len(backend.dirty) != 0 {
		t.Errorf("MarkDirty should not have been called")
	}
}

func TestWriteMetadata_PropagatesUpdateError(t *testing.T) {
	backend := &fakeBackend{updateErr: errTest}
	book := &library.ComicBook{ID: "book-1"}

	_, err := WriteMetadata(backend, book, testVolume(), testIssueDetail(), DefaultScraperConfig())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetCustomValue(t *testing.T) {
	tests := []struct {
		store, key, value, want string
	}{
		{"", "comicvine_volume", "100", ",comicvine_volume=100"},
		{",comicvine_volume=100", "comicvine_volume", "200", ",comicvine_volume=200"},
		{",a=1,comicvine_volume=100,b=2", "comicvine_volume", "200", ",a=1,comicvine_volume=200,b=2"},
	}
	for _, tt := range tests {
		if got := setCustomValue(tt.store, tt.key, tt.value); got != tt.want {
			t.Errorf("setCustomValue(%q, %q, %q) = %q, want %q", tt.store, tt.key, tt.value, got, tt.want)
		}
	}
}
