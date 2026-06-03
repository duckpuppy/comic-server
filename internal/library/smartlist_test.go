package library

import (
	"testing"
	"time"
)

// TestMatcherStringOperators tests string-based matching operators
func TestMatcherStringOperators(t *testing.T) {
	tests := []struct {
		name      string
		operator  MatchOperator
		fieldType MatcherType
		matchVal  string
		bookVal   string
		want      bool
	}{
		{
			name:      "equals - match",
			operator:  OperatorEquals,
			fieldType: MatcherTypeSeries,
			matchVal:  "Batman",
			bookVal:   "Batman",
			want:      true,
		},
		{
			name:      "equals - no match",
			operator:  OperatorEquals,
			fieldType: MatcherTypeSeries,
			matchVal:  "Batman",
			bookVal:   "Superman",
			want:      false,
		},
		{
			name:      "contains - match",
			operator:  OperatorContains,
			fieldType: MatcherTypePublisher,
			matchVal:  "Comics",
			bookVal:   "DC Comics",
			want:      true,
		},
		{
			name:      "contains - no match",
			operator:  OperatorContains,
			fieldType: MatcherTypePublisher,
			matchVal:  "Marvel",
			bookVal:   "DC Comics",
			want:      false,
		},
		{
			name:      "contains any - match first",
			operator:  OperatorContainsAny,
			fieldType: MatcherTypeTags,
			matchVal:  "action,drama,comedy",
			bookVal:   "action superhero",
			want:      true,
		},
		{
			name:      "contains any - match last",
			operator:  OperatorContainsAny,
			fieldType: MatcherTypeTags,
			matchVal:  "action,drama,comedy",
			bookVal:   "funny comedy",
			want:      true,
		},
		{
			name:      "contains any - no match",
			operator:  OperatorContainsAny,
			fieldType: MatcherTypeTags,
			matchVal:  "action,drama,comedy",
			bookVal:   "horror thriller",
			want:      false,
		},
		{
			name:      "contains all - match",
			operator:  OperatorContainsAll,
			fieldType: MatcherTypeTags,
			matchVal:  "action,superhero",
			bookVal:   "action superhero batman",
			want:      true,
		},
		{
			name:      "contains all - partial match",
			operator:  OperatorContainsAll,
			fieldType: MatcherTypeTags,
			matchVal:  "action,superhero,drama",
			bookVal:   "action superhero",
			want:      false,
		},
		{
			name:      "starts with - match",
			operator:  OperatorStartsWith,
			fieldType: MatcherTypeSeries,
			matchVal:  "Bat",
			bookVal:   "Batman",
			want:      true,
		},
		{
			name:      "starts with - no match",
			operator:  OperatorStartsWith,
			fieldType: MatcherTypeSeries,
			matchVal:  "Bat",
			bookVal:   "Superman",
			want:      false,
		},
		{
			name:      "ends with - match",
			operator:  OperatorEndsWith,
			fieldType: MatcherTypeSeries,
			matchVal:  "man",
			bookVal:   "Batman",
			want:      true,
		},
		{
			name:      "ends with - no match",
			operator:  OperatorEndsWith,
			fieldType: MatcherTypeSeries,
			matchVal:  "girl",
			bookVal:   "Batman",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{}
			switch tt.fieldType {
			case MatcherTypeSeries:
				book.Series = tt.bookVal
			case MatcherTypePublisher:
				book.Publisher = tt.bookVal
			case MatcherTypeTags:
				book.Tags = tt.bookVal
			}

			matcher := &Matcher{
				Type:       tt.fieldType,
				Operator:   tt.operator,
				MatchValue: tt.matchVal,
				IgnoreCase: true,
			}

			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Matcher.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatcherNumericOperators tests numeric-based matching operators
func TestMatcherNumericOperators(t *testing.T) {
	tests := []struct {
		name      string
		operator  MatchOperator
		fieldType MatcherType
		matchVal  string
		matchVal2 string // For range
		bookVal   int
		want      bool
	}{
		{
			name:      "year equals - match",
			operator:  OperatorEquals,
			fieldType: MatcherTypeYear,
			matchVal:  "2020",
			bookVal:   2020,
			want:      true,
		},
		{
			name:      "year equals - no match",
			operator:  OperatorEquals,
			fieldType: MatcherTypeYear,
			matchVal:  "2020",
			bookVal:   2021,
			want:      false,
		},
		{
			name:      "year greater than - match",
			operator:  OperatorGreater,
			fieldType: MatcherTypeYear,
			matchVal:  "2020",
			bookVal:   2021,
			want:      true,
		},
		{
			name:      "year greater than - no match",
			operator:  OperatorGreater,
			fieldType: MatcherTypeYear,
			matchVal:  "2020",
			bookVal:   2019,
			want:      false,
		},
		{
			name:      "year less than - match",
			operator:  OperatorLesser,
			fieldType: MatcherTypeYear,
			matchVal:  "2020",
			bookVal:   2019,
			want:      true,
		},
		{
			name:      "year less than - no match",
			operator:  OperatorLesser,
			fieldType: MatcherTypeYear,
			matchVal:  "2020",
			bookVal:   2021,
			want:      false,
		},
		{
			name:      "year in range - match",
			operator:  OperatorInRange,
			fieldType: MatcherTypeYear,
			matchVal:  "2018",
			matchVal2: "2022",
			bookVal:   2020,
			want:      true,
		},
		{
			name:      "year in range - below",
			operator:  OperatorInRange,
			fieldType: MatcherTypeYear,
			matchVal:  "2018",
			matchVal2: "2022",
			bookVal:   2017,
			want:      false,
		},
		{
			name:      "year in range - above",
			operator:  OperatorInRange,
			fieldType: MatcherTypeYear,
			matchVal:  "2018",
			matchVal2: "2022",
			bookVal:   2023,
			want:      false,
		},
		{
			name:      "page count greater than - match",
			operator:  OperatorGreater,
			fieldType: MatcherTypePageCount,
			matchVal:  "100",
			bookVal:   150,
			want:      true,
		},
		{
			name:      "page count greater than - no match",
			operator:  OperatorGreater,
			fieldType: MatcherTypePageCount,
			matchVal:  "100",
			bookVal:   50,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{}
			switch tt.fieldType {
			case MatcherTypeYear:
				book.Year = tt.bookVal
			case MatcherTypePageCount:
				book.PageCount = tt.bookVal
			}

			matcher := &Matcher{
				Type:        tt.fieldType,
				Operator:    tt.operator,
				MatchValue:  tt.matchVal,
				MatchValue2: tt.matchVal2,
			}

			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Matcher.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatcherNegation tests the Not flag (negation)
func TestMatcherNegation(t *testing.T) {
	book := &ComicBook{
		Series: "Batman",
	}

	matcher := &Matcher{
		Type:       MatcherTypeSeries,
		Operator:   OperatorEquals,
		MatchValue: "Batman",
		IgnoreCase: true,
		Not:        false,
	}

	// Without negation should match
	if !matcher.Match(book) {
		t.Error("Expected match without negation")
	}

	// With negation should not match
	matcher.Not = true
	if matcher.Match(book) {
		t.Error("Expected no match with negation")
	}
}

// TestMatcherRegex tests regex matching
func TestMatcherRegex(t *testing.T) {
	tests := []struct {
		name    string
		regex   string
		bookVal string
		want    bool
		wantErr bool
	}{
		{
			name:    "simple regex match",
			regex:   "^Bat.*",
			bookVal: "Batman",
			want:    true,
		},
		{
			name:    "simple regex no match",
			regex:   "^Bat.*",
			bookVal: "Superman",
			want:    false,
		},
		{
			name:    "complex regex match",
			regex:   "^(Batman|Superman)",
			bookVal: "Batman: The Dark Knight",
			want:    true,
		},
		{
			name:    "invalid regex",
			regex:   "[invalid",
			bookVal: "test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xmlMatcher := &ComicBookMatcher{
				Type:          "Series",
				MatchOperator: "7", // Regex operator
				MatchValue:    tt.regex,
			}

			matcher, err := NewMatcherFromXML(xmlMatcher)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error for invalid regex")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewMatcherFromXML() error = %v", err)
			}

			book := &ComicBook{Series: tt.bookVal}
			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Matcher.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatcherCaseInsensitive tests case-insensitive string matching
func TestMatcherCaseInsensitive(t *testing.T) {
	book := &ComicBook{
		Series: "Batman",
	}

	matcher := &Matcher{
		Type:       MatcherTypeSeries,
		Operator:   OperatorEquals,
		MatchValue: "batman", // lowercase
		IgnoreCase: true,
	}

	if !matcher.Match(book) {
		t.Error("Expected case-insensitive match")
	}

	matcher.IgnoreCase = false
	if matcher.Match(book) {
		t.Error("Expected case-sensitive mismatch")
	}
}

// TestMatchBooksAndMode tests smart list evaluation with AND mode
func TestMatchBooksAndMode(t *testing.T) {
	library := &ComicLibrary{
		Books: []ComicBook{
			{ID: "1", Series: "Batman", Publisher: "DC Comics", Year: 2020},
			{ID: "2", Series: "Batman", Publisher: "DC Comics", Year: 2019},
			{ID: "3", Series: "Superman", Publisher: "DC Comics", Year: 2020},
			{ID: "4", Series: "Spider-Man", Publisher: "Marvel Comics", Year: 2020},
		},
		ComicLists: []ComicListItem{
			{
				Type:        "ComicSmartListItem",
				Name:        "Recent Batman",
				MatcherMode: "And",
				Matchers: []ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
					{Type: "Year", MatchOperator: "1", MatchValue: "2019"}, // Greater than 2019
				},
			},
		},
	}

	list := &library.ComicLists[0]
	books, err := library.MatchBooks(list)
	if err != nil {
		t.Fatalf("MatchBooks() error = %v", err)
	}

	// Should match only book 1 (Batman + Year > 2019)
	if len(books) != 1 {
		t.Errorf("Expected 1 book, got %d", len(books))
	}
	if len(books) > 0 && books[0].ID != "1" {
		t.Errorf("Expected book ID 1, got %s", books[0].ID)
	}
}

// TestMatchBooksOrMode tests smart list evaluation with OR mode
func TestMatchBooksOrMode(t *testing.T) {
	library := &ComicLibrary{
		Books: []ComicBook{
			{ID: "1", Series: "Batman", Publisher: "DC Comics"},
			{ID: "2", Series: "Superman", Publisher: "DC Comics"},
			{ID: "3", Series: "Spider-Man", Publisher: "Marvel Comics"},
			{ID: "4", Series: "Wolverine", Publisher: "Marvel Comics"},
		},
		ComicLists: []ComicListItem{
			{
				Type:        "ComicSmartListItem",
				Name:        "Batman or Marvel",
				MatcherMode: "Or",
				Matchers: []ComicBookMatcher{
					{Type: "Series", MatchOperator: "0", MatchValue: "Batman"},
					{Type: "Publisher", MatchOperator: "0", MatchValue: "Marvel Comics"},
				},
			},
		},
	}

	list := &library.ComicLists[0]
	books, err := library.MatchBooks(list)
	if err != nil {
		t.Fatalf("MatchBooks() error = %v", err)
	}

	// Should match books 1, 3, 4 (Batman OR Marvel)
	if len(books) != 3 {
		t.Errorf("Expected 3 books, got %d", len(books))
	}

	// Verify we got the right books
	gotIDs := make(map[string]bool)
	for _, book := range books {
		gotIDs[book.ID] = true
	}
	wantIDs := map[string]bool{"1": true, "3": true, "4": true}
	for id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("Expected book ID %s in results", id)
		}
	}
}

// TestMatchBooksEmptyMatchers tests error handling for empty matchers
func TestMatchBooksEmptyMatchers(t *testing.T) {
	library := &ComicLibrary{
		Books: []ComicBook{
			{ID: "1", Series: "Batman"},
		},
		ComicLists: []ComicListItem{
			{
				Type:        "ComicSmartListItem",
				Name:        "Empty List",
				MatcherMode: "And",
				Matchers:    []ComicBookMatcher{}, // No matchers
			},
		},
	}

	list := &library.ComicLists[0]
	_, err := library.MatchBooks(list)
	if err == nil {
		t.Error("Expected error for empty matchers")
	}
}

// TestMatchBooksNotSmartList tests error handling for non-smart lists
func TestMatchBooksNotSmartList(t *testing.T) {
	library := &ComicLibrary{
		Books: []ComicBook{
			{ID: "1", Series: "Batman"},
		},
		ComicLists: []ComicListItem{
			{
				Type: "ComicReadingList", // Not a smart list
				Name: "Reading List",
			},
		},
	}

	list := &library.ComicLists[0]
	_, err := library.MatchBooks(list)
	if err == nil {
		t.Error("Expected error for non-smart list")
	}
}

// TestMatcherYesNo tests Yes/No/Unknown matching
func TestMatcherYesNo(t *testing.T) {
	tests := []struct {
		name     string
		operator MatchOperator
		bookVal  string
		want     bool
	}{
		{
			name:     "equals yes - match",
			operator: OperatorEqualsYes,
			bookVal:  "Yes",
			want:     true,
		},
		{
			name:     "equals yes - no match",
			operator: OperatorEqualsYes,
			bookVal:  "No",
			want:     false,
		},
		{
			name:     "equals no - match",
			operator: OperatorEqualsNo,
			bookVal:  "No",
			want:     true,
		},
		{
			name:     "equals no - no match",
			operator: OperatorEqualsNo,
			bookVal:  "Yes",
			want:     false,
		},
		{
			name:     "equals unknown - match",
			operator: OperatorEqualsUnknown,
			bookVal:  "Unknown",
			want:     true,
		},
		{
			name:     "equals unknown - empty",
			operator: OperatorEqualsUnknown,
			bookVal:  "",
			want:     true,
		},
		{
			name:     "equals unknown - no match",
			operator: OperatorEqualsUnknown,
			bookVal:  "Yes",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{
				SeriesComplete: tt.bookVal,
			}

			matcher := &Matcher{
				Type:     MatcherTypeSeriesComplete,
				Operator: tt.operator,
			}

			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Matcher.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatcherDate tests date matching
func TestMatcherDate(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)
	weekAgo := now.AddDate(0, 0, -7)

	tests := []struct {
		name      string
		operator  MatchOperator
		matchVal  string
		matchVal2 string
		bookTime  time.Time
		want      bool
	}{
		{
			name:     "is after - match",
			operator: OperatorIsAfter,
			matchVal: yesterday.Format(time.RFC3339),
			bookTime: now,
			want:     true,
		},
		{
			name:     "is after - no match",
			operator: OperatorIsAfter,
			matchVal: tomorrow.Format(time.RFC3339),
			bookTime: now,
			want:     false,
		},
		{
			name:     "is before - match",
			operator: OperatorIsBefore,
			matchVal: tomorrow.Format(time.RFC3339),
			bookTime: now,
			want:     true,
		},
		{
			name:     "is before - no match",
			operator: OperatorIsBefore,
			matchVal: yesterday.Format(time.RFC3339),
			bookTime: now,
			want:     false,
		},
		{
			name:     "in last 7 days - match",
			operator: OperatorIsInLastDays,
			matchVal: "7",
			bookTime: time.Now().AddDate(0, 0, -3), // Use fresh time.Now()
			want:     true,
		},
		{
			name:     "in last 7 days - no match",
			operator: OperatorIsInLastDays,
			matchVal: "7",
			bookTime: weekAgo.AddDate(0, 0, -3), // 10 days ago
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{}
			book.AddedTime = ComicTime{tt.bookTime}

			matcher := &Matcher{
				Type:        MatcherTypeAddedTime,
				Operator:    tt.operator,
				MatchValue:  tt.matchVal,
				MatchValue2: tt.matchVal2,
			}

			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Matcher.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewMatcherFromXML tests creating matchers from XML data
func TestNewMatcherFromXML(t *testing.T) {
	tests := []struct {
		name        string
		xmlMatcher  *ComicBookMatcher
		wantErr     bool
		checkResult func(*testing.T, *Matcher)
	}{
		{
			name: "string matcher with numeric operator",
			xmlMatcher: &ComicBookMatcher{
				Type:          "Series",
				MatchOperator: "0", // Equals
				MatchValue:    "Batman",
			},
			wantErr: false,
			checkResult: func(t *testing.T, m *Matcher) {
				if m.Type != MatcherTypeSeries {
					t.Errorf("Type = %v, want Series", m.Type)
				}
				if m.Operator != OperatorEquals {
					t.Errorf("Operator = %v, want OperatorEquals", m.Operator)
				}
				if m.MatchValue != "Batman" {
					t.Errorf("MatchValue = %v, want Batman", m.MatchValue)
				}
			},
		},
		{
			name: "string matcher with text operator",
			xmlMatcher: &ComicBookMatcher{
				Type:          "Publisher",
				MatchOperator: "contains",
				MatchValue:    "Comics",
			},
			wantErr: false,
			checkResult: func(t *testing.T, m *Matcher) {
				if m.Operator != OperatorContains {
					t.Errorf("Operator = %v, want OperatorContains", m.Operator)
				}
			},
		},
		{
			name: "invalid operator",
			xmlMatcher: &ComicBookMatcher{
				Type:          "Series",
				MatchOperator: "invalid_operator",
				MatchValue:    "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewMatcherFromXML(tt.xmlMatcher)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMatcherFromXML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil {
				tt.checkResult(t, matcher)
			}
		})
	}
}

// TestCustomValuesMatcher tests the CustomValues matcher including key-existence checks
func TestCustomValuesMatcher(t *testing.T) {
	tests := []struct {
		name       string
		store      string
		matchKey   string
		matchVal   string
		not        bool
		want       bool
	}{
		{
			name:     "key exists - match",
			store:    ",comicvine_issue=133658,comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			want:     true,
		},
		{
			name:     "key not present - no match",
			store:    ",comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			want:     false,
		},
		{
			name:     "NOT key exists - filters book with key",
			store:    ",comicvine_issue=133658,comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			not:      true,
			want:     false,
		},
		{
			name:     "NOT key exists - passes book without key",
			store:    ",comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			not:      true,
			want:     true,
		},
		{
			name:     "key+value match",
			store:    ",comicvine_issue=133658",
			matchKey: "comicvine_issue",
			matchVal: "133658",
			want:     true,
		},
		{
			name:     "key+value mismatch",
			store:    ",comicvine_issue=133658",
			matchKey: "comicvine_issue",
			matchVal: "999999",
			want:     false,
		},
		{
			name:     "empty store - no match",
			store:    "",
			matchKey: "comicvine_issue",
			matchVal: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{CustomValuesStore: tt.store}
			xmlM := &ComicBookMatcher{
				Type:        "ComicBookCustomValuesMatcher",
				MatchValue:  tt.matchKey,
				MatchValue2: tt.matchVal,
				Not:         tt.not,
			}
			matcher, err := NewMatcherFromXML(xmlM)
			if err != nil {
				t.Fatalf("NewMatcherFromXML error: %v", err)
			}
			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestListContainsMatcher tests MatchOperator=6 (ListContains) for tag-like fields
func TestListContainsMatcher(t *testing.T) {
	tests := []struct {
		name     string
		tags     string
		matchVal string
		not      bool
		want     bool
	}{
		{
			name:     "tag present",
			tags:     "Action, NOSCRAPE, Superhero",
			matchVal: "NOSCRAPE",
			want:     true,
		},
		{
			name:     "tag absent",
			tags:     "Action, Superhero",
			matchVal: "NOSCRAPE",
			want:     false,
		},
		{
			name:     "NOT tag present - filters out",
			tags:     "Action, NOSCRAPE",
			matchVal: "NOSCRAPE",
			not:      true,
			want:     false,
		},
		{
			name:     "NOT tag absent - passes through",
			tags:     "Action, Superhero",
			matchVal: "NOSCRAPE",
			not:      true,
			want:     true,
		},
		{
			name:     "case insensitive match",
			tags:     "noscrape, Action",
			matchVal: "NOSCRAPE",
			want:     true,
		},
		{
			name:     "partial match does not count",
			tags:     "NOSCRAPE_EXTRA, Action",
			matchVal: "NOSCRAPE",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{Tags: tt.tags}
			xmlM := &ComicBookMatcher{
				Type:          "ComicBookTagsMatcher",
				MatchOperator: "6",
				MatchValue:    tt.matchVal,
				Not:           tt.not,
			}
			matcher, err := NewMatcherFromXML(xmlM)
			if err != nil {
				t.Fatalf("NewMatcherFromXML error: %v", err)
			}
			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPathMatcherWindowsPaths verifies that Windows paths in library XML work on Linux
func TestPathMatcherWindowsPaths(t *testing.T) {
	book := &ComicBook{FilePath: `G:\Comics\Marvel\Spider-Man #001.cbz`}

	t.Run("directory starts with Windows root", func(t *testing.T) {
		xmlM := &ComicBookMatcher{
			Type:          "ComicBookDirectoryMatcher",
			MatchOperator: "4", // StartsWith
			MatchValue:    `G:\Comics\`,
		}
		m, err := NewMatcherFromXML(xmlM)
		if err != nil {
			t.Fatalf("NewMatcherFromXML error: %v", err)
		}
		if !m.Match(book) {
			t.Error("expected directory to match Windows root path prefix")
		}
	})

	t.Run("NOT directory starts with root - book in root does not match", func(t *testing.T) {
		xmlM := &ComicBookMatcher{
			Type:          "ComicBookDirectoryMatcher",
			Not:           true,
			MatchOperator: "4",
			MatchValue:    `G:\Comics\`,
		}
		m, err := NewMatcherFromXML(xmlM)
		if err != nil {
			t.Fatalf("NewMatcherFromXML error: %v", err)
		}
		if m.Match(book) {
			t.Error("book in G:\\Comics\\ should not match NOT-StartsWith G:\\Comics\\")
		}
	})

	t.Run("filename extraction from Windows path", func(t *testing.T) {
		xmlM := &ComicBookMatcher{
			Type:          "ComicBookFileMatcher",
			MatchOperator: "1", // Contains
			MatchValue:    "Spider-Man",
		}
		m, err := NewMatcherFromXML(xmlM)
		if err != nil {
			t.Fatalf("NewMatcherFromXML error: %v", err)
		}
		if !m.Match(book) {
			t.Error("expected filename to contain 'Spider-Man'")
		}
	})
}
