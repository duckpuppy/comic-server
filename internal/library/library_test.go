package library

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleLibraryXML = `<?xml version="1.0" encoding="utf-8"?>
<ComicDatabase Id="12345678-1234-1234-1234-123456789abc">
  <Name>My Comic Library</Name>
  <Books>
    <Book Id="aaaaaaaa-bbbb-cccc-dddd-111111111111" File="/comics/Batman #1.cbr">
      <Title>The Court of Owls</Title>
      <Series>Batman</Series>
      <Number>1</Number>
      <Volume>2</Volume>
      <Count>52</Count>
      <Year>2011</Year>
      <Month>9</Month>
      <Publisher>DC Comics</Publisher>
      <Imprint>The New 52</Imprint>
      <Format>Comic</Format>
      <Writer>Scott Snyder</Writer>
      <Penciller>Greg Capullo</Penciller>
      <Inker>Jonathan Glapion</Inker>
      <PageCount>32</PageCount>
      <Rating>4.5</Rating>
      <CurrentPage>5</CurrentPage>
      <OpenCount>3</OpenCount>
      <Added>2024-01-15T10:30:00</Added>
      <Pages>
        <Page Image="0" Type="FrontCover" ImageWidth="1280" ImageHeight="1920" />
        <Page Image="1" Type="Story" ImageWidth="1280" ImageHeight="1920" />
        <Page Image="2" Type="Story" ImageWidth="1280" ImageHeight="1920" />
      </Pages>
    </Book>
    <Book Id="bbbbbbbb-cccc-dddd-eeee-222222222222" File="/comics/Spider-Man #1.cbz">
      <Title>Coming Home</Title>
      <Series>The Amazing Spider-Man</Series>
      <Number>1</Number>
      <Volume>2</Volume>
      <Year>1999</Year>
      <Month>1</Month>
      <Publisher>Marvel Comics</Publisher>
      <Writer>Howard Mackie</Writer>
      <Penciller>John Byrne</Penciller>
      <PageCount>38</PageCount>
      <Rating>3.0</Rating>
      <CurrentPage>0</CurrentPage>
      <AddedTime>2024-02-01T14:20:00</AddedTime>
    </Book>
    <Book Id="cccccccc-dddd-eeee-ffff-333333333333" File="/comics/Saga #1.cbz">
      <Title>Chapter One</Title>
      <Series>Saga</Series>
      <Number>1</Number>
      <Volume>1</Volume>
      <Year>2012</Year>
      <Month>3</Month>
      <Publisher>Image Comics</Publisher>
      <Writer>Brian K. Vaughan</Writer>
      <Penciller>Fiona Staples</Penciller>
      <PageCount>44</PageCount>
      <Rating>5.0</Rating>
      <CurrentPage>44</CurrentPage>
      <LastPageRead>44</LastPageRead>
      <AddedTime>2024-01-20T09:15:00</AddedTime>
    </Book>
  </Books>
  <ComicLists>
    <Item xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="ComicLibraryListItem" Id="library-root" Name="Library">
      <BookCount>3</BookCount>
    </Item>
    <Item xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="ComicReadingList" Id="dddddddd-eeee-ffff-0000-444444444444" Name="To Read">
      <Description>Books I want to read next</Description>
      <BookCount>2</BookCount>
      <Books>
        <Book Series="Batman" Number="1" Volume="2" Year="2011" Format="Comic" Id="aaaaaaaa-bbbb-cccc-dddd-111111111111" />
        <Book Series="The Amazing Spider-Man" Number="1" Volume="2" Year="1999" Id="bbbbbbbb-cccc-dddd-eeee-222222222222" />
      </Books>
    </Item>
    <Item xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="ComicSmartListItem" Id="eeeeeeee-ffff-0000-1111-555555555555" Name="Favorites" MatcherMode="And">
      <Description>Highly rated comics</Description>
      <Matchers>
        <ComicBookMatcher xsi:type="ComicBookRatingMatcher" MatchOperator="1">
          <MatchValue>4</MatchValue>
        </ComicBookMatcher>
      </Matchers>
    </Item>
    <Item xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="ComicListItemFolder" Id="ffffffff-0000-1111-2222-666666666666" Name="Publishers">
      <Collapsed>false</Collapsed>
      <Items>
        <Item xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="ComicSmartListItem" Id="00000000-1111-2222-3333-777777777777" Name="DC Comics" MatcherMode="And">
          <Matchers>
            <ComicBookMatcher xsi:type="ComicBookPublisherMatcher" MatchOperator="0">
              <MatchValue>DC Comics</MatchValue>
            </ComicBookMatcher>
          </Matchers>
        </Item>
      </Items>
    </Item>
  </ComicLists>
</ComicDatabase>`

func TestLoadLibrary(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-library.xml")

	if err := os.WriteFile(testFile, []byte(sampleLibraryXML), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Load the library
	library, err := LoadLibrary(testFile)
	if err != nil {
		t.Fatalf("LoadLibrary() error = %v", err)
	}

	// Verify library metadata
	if library.ID != "12345678-1234-1234-1234-123456789abc" {
		t.Errorf("Library ID = %v, want %v", library.ID, "12345678-1234-1234-1234-123456789abc")
	}
	if library.Name != "My Comic Library" {
		t.Errorf("Library Name = %v, want %v", library.Name, "My Comic Library")
	}

	// Verify book count
	if len(library.Books) != 3 {
		t.Fatalf("Books count = %d, want 3", len(library.Books))
	}

	// Verify list count
	if len(library.ComicLists) != 4 {
		t.Fatalf("ComicLists count = %d, want 4", len(library.ComicLists))
	}
}

func TestComicBookParsing(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	tests := []struct {
		name       string
		bookIndex  int
		wantID     string
		wantTitle  string
		wantSeries string
		wantNumber string
		wantVolume int
		wantYear   int
		wantPages  int
	}{
		{
			name:       "Batman #1",
			bookIndex:  0,
			wantID:     "aaaaaaaa-bbbb-cccc-dddd-111111111111",
			wantTitle:  "The Court of Owls",
			wantSeries: "Batman",
			wantNumber: "1",
			wantVolume: 2,
			wantYear:   2011,
			wantPages:  32,
		},
		{
			name:       "Spider-Man #1",
			bookIndex:  1,
			wantID:     "bbbbbbbb-cccc-dddd-eeee-222222222222",
			wantTitle:  "Coming Home",
			wantSeries: "The Amazing Spider-Man",
			wantNumber: "1",
			wantVolume: 2,
			wantYear:   1999,
			wantPages:  38,
		},
		{
			name:       "Saga #1",
			bookIndex:  2,
			wantID:     "cccccccc-dddd-eeee-ffff-333333333333",
			wantTitle:  "Chapter One",
			wantSeries: "Saga",
			wantNumber: "1",
			wantVolume: 1,
			wantYear:   2012,
			wantPages:  44,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := library.Books[tt.bookIndex]

			if book.ID != tt.wantID {
				t.Errorf("ID = %v, want %v", book.ID, tt.wantID)
			}
			if book.Title != tt.wantTitle {
				t.Errorf("Title = %v, want %v", book.Title, tt.wantTitle)
			}
			if book.Series != tt.wantSeries {
				t.Errorf("Series = %v, want %v", book.Series, tt.wantSeries)
			}
			if book.Number != tt.wantNumber {
				t.Errorf("Number = %v, want %v", book.Number, tt.wantNumber)
			}
			if book.Volume != tt.wantVolume {
				t.Errorf("Volume = %v, want %v", book.Volume, tt.wantVolume)
			}
			if book.Year != tt.wantYear {
				t.Errorf("Year = %v, want %v", book.Year, tt.wantYear)
			}
			if book.PageCount != tt.wantPages {
				t.Errorf("PageCount = %v, want %v", book.PageCount, tt.wantPages)
			}
		})
	}
}

func TestComicBookMetadata(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	batman := library.Books[0]

	// Test creative credits
	if batman.Writer != "Scott Snyder" {
		t.Errorf("Writer = %v, want Scott Snyder", batman.Writer)
	}
	if batman.Penciller != "Greg Capullo" {
		t.Errorf("Penciller = %v, want Greg Capullo", batman.Penciller)
	}
	if batman.Inker != "Jonathan Glapion" {
		t.Errorf("Inker = %v, want Jonathan Glapion", batman.Inker)
	}

	// Test publishing info
	if batman.Publisher != "DC Comics" {
		t.Errorf("Publisher = %v, want DC Comics", batman.Publisher)
	}
	if batman.Imprint != "The New 52" {
		t.Errorf("Imprint = %v, want The New 52", batman.Imprint)
	}
	if batman.Format != "Comic" {
		t.Errorf("Format = %v, want Comic", batman.Format)
	}
	if batman.Month != 9 {
		t.Errorf("Month = %v, want 9", batman.Month)
	}

	// Test user tracking
	if batman.Rating != 4.5 {
		t.Errorf("Rating = %v, want 4.5", batman.Rating)
	}
	if batman.CurrentPage != 5 {
		t.Errorf("CurrentPage = %v, want 5", batman.CurrentPage)
	}
	if batman.OpenCount != 3 {
		t.Errorf("OpenCount = %v, want 3", batman.OpenCount)
	}

	// Test file path
	if batman.FilePath != "/comics/Batman #1.cbr" {
		t.Errorf("FilePath = %v, want /comics/Batman #1.cbr", batman.FilePath)
	}
}

func TestComicBookPages(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	batman := library.Books[0]

	if len(batman.Pages) != 3 {
		t.Fatalf("Pages count = %d, want 3", len(batman.Pages))
	}

	// Test front cover page
	frontCover := batman.Pages[0]
	if frontCover.Image != 0 {
		t.Errorf("Page 0 Image = %v, want 0", frontCover.Image)
	}
	if frontCover.Type != "FrontCover" {
		t.Errorf("Page 0 Type = %v, want FrontCover", frontCover.Type)
	}
	if frontCover.ImageWidth != 1280 {
		t.Errorf("Page 0 ImageWidth = %v, want 1280", frontCover.ImageWidth)
	}
	if frontCover.ImageHeight != 1920 {
		t.Errorf("Page 0 ImageHeight = %v, want 1920", frontCover.ImageHeight)
	}

	// Test story pages
	storyPage := batman.Pages[1]
	if storyPage.Type != "Story" {
		t.Errorf("Page 1 Type = %v, want Story", storyPage.Type)
	}
}

func TestReadingListParsing(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Find "To Read" list (index 1)
	toRead := library.ComicLists[1]

	if toRead.Name != "To Read" {
		t.Errorf("List name = %v, want To Read", toRead.Name)
	}
	if toRead.Description != "Books I want to read next" {
		t.Errorf("List description = %v, want 'Books I want to read next'", toRead.Description)
	}
	if toRead.BookCount != 2 {
		t.Errorf("BookCount = %v, want 2", toRead.BookCount)
	}

	// Check list items
	if len(toRead.Items) != 2 {
		t.Fatalf("Items count = %d, want 2", len(toRead.Items))
	}

	// Verify first item
	item1 := toRead.Items[0]
	if item1.Series != "Batman" {
		t.Errorf("Item 1 Series = %v, want Batman", item1.Series)
	}
	if item1.Number != "1" {
		t.Errorf("Item 1 Number = %v, want 1", item1.Number)
	}
	if item1.Volume != 2 {
		t.Errorf("Item 1 Volume = %v, want 2", item1.Volume)
	}
	if item1.Year != 2011 {
		t.Errorf("Item 1 Year = %v, want 2011", item1.Year)
	}
	if item1.ID != "aaaaaaaa-bbbb-cccc-dddd-111111111111" {
		t.Errorf("Item 1 ID = %v, want aaaaaaaa-bbbb-cccc-dddd-111111111111", item1.ID)
	}
}

func TestSmartListParsing(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Find "Favorites" smart list (index 2)
	favorites := library.ComicLists[2]

	if favorites.Name != "Favorites" {
		t.Errorf("List name = %v, want Favorites", favorites.Name)
	}
	if favorites.MatcherMode != "And" {
		t.Errorf("MatcherMode = %v, want And", favorites.MatcherMode)
	}

	// Check matchers
	if len(favorites.Matchers) != 1 {
		t.Fatalf("Matchers count = %d, want 1", len(favorites.Matchers))
	}

	matcher := favorites.Matchers[0]
	if !strings.Contains(matcher.Type, "Rating") {
		t.Errorf("Matcher Type = %v, want to contain 'Rating'", matcher.Type)
	}
	if matcher.MatchOperator != "1" {
		t.Errorf("Matcher Operator = %v, want 1", matcher.MatchOperator)
	}
	if matcher.MatchValue != "4" {
		t.Errorf("Matcher MatchValue = %v, want 4", matcher.MatchValue)
	}
}

func TestFolderParsing(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Find "Publishers" folder (index 3)
	publishers := library.ComicLists[3]

	if publishers.Name != "Publishers" {
		t.Errorf("Folder name = %v, want Publishers", publishers.Name)
	}
	if publishers.Collapsed != false {
		t.Errorf("Collapsed = %v, want false", publishers.Collapsed)
	}

	// Check child items
	if len(publishers.ChildItems) != 1 {
		t.Fatalf("ChildItems count = %d, want 1", len(publishers.ChildItems))
	}

	dcList := publishers.ChildItems[0]
	if dcList.Name != "DC Comics" {
		t.Errorf("Child list name = %v, want DC Comics", dcList.Name)
	}
}

func TestGetBook(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	tests := []struct {
		name      string
		id        string
		wantTitle string
		wantNil   bool
	}{
		{
			name:      "Find Batman",
			id:        "aaaaaaaa-bbbb-cccc-dddd-111111111111",
			wantTitle: "The Court of Owls",
			wantNil:   false,
		},
		{
			name:      "Find Saga",
			id:        "cccccccc-dddd-eeee-ffff-333333333333",
			wantTitle: "Chapter One",
			wantNil:   false,
		},
		{
			name:    "Not found",
			id:      "nonexistent-id",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := library.GetBook(tt.id)

			if tt.wantNil {
				if book != nil {
					t.Errorf("GetBook() = %v, want nil", book)
				}
				return
			}

			if book == nil {
				t.Fatalf("GetBook() = nil, want book")
			}
			if book.Title != tt.wantTitle {
				t.Errorf("Title = %v, want %v", book.Title, tt.wantTitle)
			}
		})
	}
}

func TestGetBooksByList(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Get "To Read" list
	toRead := &library.ComicLists[1]
	books := library.GetBooksByList(toRead)

	if len(books) != 2 {
		t.Fatalf("GetBooksByList() returned %d books, want 2", len(books))
	}

	// Verify books
	if books[0].Series != "Batman" {
		t.Errorf("Book 0 Series = %v, want Batman", books[0].Series)
	}
	if books[1].Series != "The Amazing Spider-Man" {
		t.Errorf("Book 1 Series = %v, want The Amazing Spider-Man", books[1].Series)
	}
}

func TestFindList(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	tests := []struct {
		name     string
		listName string
		wantID   string
		wantNil  bool
	}{
		{
			name:     "Find To Read",
			listName: "To Read",
			wantID:   "dddddddd-eeee-ffff-0000-444444444444",
			wantNil:  false,
		},
		{
			name:     "Find nested DC Comics",
			listName: "DC Comics",
			wantID:   "00000000-1111-2222-3333-777777777777",
			wantNil:  false,
		},
		{
			name:     "Not found",
			listName: "Nonexistent List",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := library.FindList(tt.listName)

			if tt.wantNil {
				if list != nil {
					t.Errorf("FindList() = %v, want nil", list)
				}
				return
			}

			if list == nil {
				t.Fatalf("FindList() = nil, want list")
			}
			if list.ID != tt.wantID {
				t.Errorf("ID = %v, want %v", list.ID, tt.wantID)
			}
		})
	}
}

func TestDateTimeParsing(t *testing.T) {
	var library ComicLibrary
	if err := xml.Unmarshal([]byte(sampleLibraryXML), &library); err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	batman := library.Books[0]

	expectedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !batman.AddedTime.Equal(expectedTime) {
		t.Errorf("AddedTime = %v, want %v", batman.AddedTime, expectedTime)
	}
}
