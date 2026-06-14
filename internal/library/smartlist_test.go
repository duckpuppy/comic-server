package library

import (
	"strconv"
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

// TestYesNoMatchers tests all Yes/No matcher types beyond SeriesComplete
func TestYesNoMatchers(t *testing.T) {
	tests := []struct {
		name        string
		matcherType MatcherType
		operator    MatchOperator
		book        *ComicBook
		want        bool
	}{
		// BlackAndWhite — string field already in "Yes"/"No"/"Unknown" format
		{name: "BlackAndWhite yes match", matcherType: MatcherTypeBlackAndWhite, operator: OperatorEqualsYes, book: &ComicBook{BlackAndWhite: "Yes"}, want: true},
		{name: "BlackAndWhite yes no match", matcherType: MatcherTypeBlackAndWhite, operator: OperatorEqualsYes, book: &ComicBook{BlackAndWhite: "No"}, want: false},
		{name: "BlackAndWhite unknown match", matcherType: MatcherTypeBlackAndWhite, operator: OperatorEqualsUnknown, book: &ComicBook{BlackAndWhite: "Unknown"}, want: true},
		{name: "BlackAndWhite unknown empty", matcherType: MatcherTypeBlackAndWhite, operator: OperatorEqualsUnknown, book: &ComicBook{}, want: true},

		// Checked — bool, no unknown state
		{name: "Checked true yes", matcherType: MatcherTypeChecked, operator: OperatorEqualsYes, book: &ComicBook{Checked: true}, want: true},
		{name: "Checked false yes", matcherType: MatcherTypeChecked, operator: OperatorEqualsYes, book: &ComicBook{Checked: false}, want: false},
		{name: "Checked false no", matcherType: MatcherTypeChecked, operator: OperatorEqualsNo, book: &ComicBook{Checked: false}, want: true},
		{name: "Checked unknown never", matcherType: MatcherTypeChecked, operator: OperatorEqualsUnknown, book: &ComicBook{Checked: true}, want: false},

		// HasCustomValues — computed from CustomValuesStore
		{name: "HasCustomValues with values", matcherType: MatcherTypeHasCustomValues, operator: OperatorEqualsYes, book: &ComicBook{CustomValuesStore: ",key=val"}, want: true},
		{name: "HasCustomValues empty", matcherType: MatcherTypeHasCustomValues, operator: OperatorEqualsYes, book: &ComicBook{}, want: false},
		{name: "HasCustomValues no match", matcherType: MatcherTypeHasCustomValues, operator: OperatorEqualsNo, book: &ComicBook{}, want: true},

		// IsLinked — computed from FilePath
		{name: "IsLinked with path", matcherType: MatcherTypeIsLinked, operator: OperatorEqualsYes, book: &ComicBook{FilePath: "/comics/book.cbz"}, want: true},
		{name: "IsLinked no path", matcherType: MatcherTypeIsLinked, operator: OperatorEqualsYes, book: &ComicBook{}, want: false},
		{name: "IsLinked not linked is no", matcherType: MatcherTypeIsLinked, operator: OperatorEqualsNo, book: &ComicBook{}, want: true},

		// IsMissing — bool field FileIsMissing
		{name: "IsMissing true yes", matcherType: MatcherTypeIsMissing, operator: OperatorEqualsYes, book: &ComicBook{FileIsMissing: true}, want: true},
		{name: "IsMissing false yes", matcherType: MatcherTypeIsMissing, operator: OperatorEqualsYes, book: &ComicBook{FileIsMissing: false}, want: false},
		{name: "IsMissing false no", matcherType: MatcherTypeIsMissing, operator: OperatorEqualsNo, book: &ComicBook{FileIsMissing: false}, want: true},

		// ModifiedInfo — bool field ComicInfoIsDirty
		{name: "ModifiedInfo true yes", matcherType: MatcherTypeModifiedInfo, operator: OperatorEqualsYes, book: &ComicBook{ComicInfoIsDirty: true}, want: true},
		{name: "ModifiedInfo false yes", matcherType: MatcherTypeModifiedInfo, operator: OperatorEqualsYes, book: &ComicBook{ComicInfoIsDirty: false}, want: false},
		{name: "ModifiedInfo false no", matcherType: MatcherTypeModifiedInfo, operator: OperatorEqualsNo, book: &ComicBook{ComicInfoIsDirty: false}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &Matcher{
				Type:     tt.matcherType,
				Operator: tt.operator,
			}
			got := matcher.Match(tt.book)
			if got != tt.want {
				t.Errorf("Matcher.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCreditsMatchers verifies each credits field is correctly extracted and matched
func TestCreditsMatchers(t *testing.T) {
	tests := []struct {
		matcherType MatcherType
		book        *ComicBook
	}{
		{MatcherTypeColorist, &ComicBook{Colorist: "Dave Stewart"}},
		{MatcherTypeCoverArtist, &ComicBook{CoverArtist: "Alex Ross"}},
		{MatcherTypeEditor, &ComicBook{Editor: "Karen Berger"}},
		{MatcherTypeInker, &ComicBook{Inker: "Klaus Janson"}},
		{MatcherTypeLetterer, &ComicBook{Letterer: "Todd Klein"}},
		{MatcherTypePenciller, &ComicBook{Penciller: "Neal Adams"}},
		{MatcherTypeTranslator, &ComicBook{Translator: "Nora Stevens Heath"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.matcherType), func(t *testing.T) {
			// Extract the field value by doing an Equals match against the known value
			var fieldVal string
			switch tt.matcherType {
			case MatcherTypeColorist:
				fieldVal = tt.book.Colorist
			case MatcherTypeCoverArtist:
				fieldVal = tt.book.CoverArtist
			case MatcherTypeEditor:
				fieldVal = tt.book.Editor
			case MatcherTypeInker:
				fieldVal = tt.book.Inker
			case MatcherTypeLetterer:
				fieldVal = tt.book.Letterer
			case MatcherTypePenciller:
				fieldVal = tt.book.Penciller
			case MatcherTypeTranslator:
				fieldVal = tt.book.Translator
			}

			matcher := &Matcher{Type: tt.matcherType, Operator: OperatorEquals, IgnoreCase: true, MatchValue: fieldVal}
			if !matcher.Match(tt.book) {
				t.Errorf("expected match for %s, got false", tt.matcherType)
			}
			// Verify no match against empty book
			if matcher.Match(&ComicBook{}) {
				t.Errorf("expected no match for empty book on %s", tt.matcherType)
			}
		})
	}
}

// TestCharacterLocationMatchers verifies MainCharacterOrTeam and Locations field extraction
func TestCharacterLocationMatchers(t *testing.T) {
	tests := []struct {
		matcherType MatcherType
		value       string
		book        *ComicBook
	}{
		{MatcherTypeMainCharacterOrTeam, "Batman", &ComicBook{MainCharacterOrTeam: "Batman"}},
		{MatcherTypeLocations, "Gotham City", &ComicBook{Locations: "Gotham City"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.matcherType), func(t *testing.T) {
			matcher := &Matcher{Type: tt.matcherType, Operator: OperatorEquals, IgnoreCase: true, MatchValue: tt.value}
			if !matcher.Match(tt.book) {
				t.Errorf("expected match for %s", tt.matcherType)
			}
			if matcher.Match(&ComicBook{}) {
				t.Errorf("expected no match for empty book on %s", tt.matcherType)
			}
		})
	}
}

// TestContentMatchers verifies StoryArc, AgeRating, Summary, Review field extraction
func TestContentMatchers(t *testing.T) {
	tests := []struct {
		matcherType MatcherType
		value       string
		book        *ComicBook
	}{
		{MatcherTypeStoryArc, "Knightfall", &ComicBook{StoryArc: "Knightfall"}},
		{MatcherTypeAgeRating, "Teen", &ComicBook{AgeRating: "Teen"}},
		{MatcherTypeSummary, "A dark tale", &ComicBook{Summary: "A dark tale"}},
		{MatcherTypeReview, "Excellent", &ComicBook{Review: "Excellent"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.matcherType), func(t *testing.T) {
			matcher := &Matcher{Type: tt.matcherType, Operator: OperatorEquals, IgnoreCase: true, MatchValue: tt.value}
			if !matcher.Match(tt.book) {
				t.Errorf("expected match for %s", tt.matcherType)
			}
			if matcher.Match(&ComicBook{}) {
				t.Errorf("expected no match for empty book on %s", tt.matcherType)
			}
		})
	}
}

// TestReadPercentageMatcher tests ReadPercentage computation and matching
func TestReadPercentageMatcher(t *testing.T) {
	tests := []struct {
		name      string
		book      *ComicBook
		operator  MatchOperator
		matchVal  string
		matchVal2 string
		want      bool
	}{
		// Unread: OpenCount==0 or LastPageRead==0 → 0%
		{name: "unread book equals 0", book: &ComicBook{PageCount: 10, LastPageRead: 0}, operator: OperatorEquals, matchVal: "0", want: true},
		{name: "no pages equals 0", book: &ComicBook{PageCount: 0, LastPageRead: 5}, operator: OperatorEquals, matchVal: "0", want: true},
		// Halfway: LastPageRead=4 of 10 pages → (4+1)*100/10 = 50%
		{name: "half read equals 50", book: &ComicBook{PageCount: 10, LastPageRead: 4}, operator: OperatorEquals, matchVal: "50", want: true},
		{name: "half read greater than 25", book: &ComicBook{PageCount: 10, LastPageRead: 4}, operator: OperatorGreater, matchVal: "25", want: true},
		{name: "half read less than 75", book: &ComicBook{PageCount: 10, LastPageRead: 4}, operator: OperatorLesser, matchVal: "75", want: true},
		// Fully read: LastPageRead=9 of 10 → (9+1)*100/10 = 100%
		{name: "fully read equals 100", book: &ComicBook{PageCount: 10, LastPageRead: 9}, operator: OperatorEquals, matchVal: "100", want: true},
		{name: "fully read greater than 50", book: &ComicBook{PageCount: 10, LastPageRead: 9}, operator: OperatorGreater, matchVal: "50", want: true},
		// Range: 25-75%
		{name: "half read in range 25-75", book: &ComicBook{PageCount: 10, LastPageRead: 4}, operator: OperatorInRange, matchVal: "25", matchVal2: "75", want: true},
		{name: "unread not in range 25-75", book: &ComicBook{PageCount: 10, LastPageRead: 0}, operator: OperatorInRange, matchVal: "25", matchVal2: "75", want: false},
		// Clamped to 100 even if arithmetic exceeds it
		{name: "clamped to 100", book: &ComicBook{PageCount: 10, LastPageRead: 99}, operator: OperatorEquals, matchVal: "100", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &Matcher{
				Type:        MatcherTypeReadPercentage,
				Operator:    tt.operator,
				MatchValue:  tt.matchVal,
				MatchValue2: tt.matchVal2,
			}
			got := matcher.Match(tt.book)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMangaMatcher tests the 4-way Manga enum matcher
func TestMangaMatcher(t *testing.T) {
	tests := []struct {
		name     string
		operator MatchOperator
		manga    string
		want     bool
	}{
		// Operator 0: is Yes (right-to-left)
		{name: "yes RTL match", operator: 0, manga: "Yes", want: true},
		{name: "yes RTL no match LTR", operator: 0, manga: "YesRightToLeft", want: false},
		{name: "yes RTL no match No", operator: 0, manga: "No", want: false},
		// Operator 1: is Yes, Left to Right
		{name: "yes LTR match YesRightToLeft", operator: 1, manga: "YesRightToLeft", want: true},
		{name: "yes LTR match YesAndRightToLeft", operator: 1, manga: "YesAndRightToLeft", want: true},
		{name: "yes LTR no match Yes", operator: 1, manga: "Yes", want: false},
		// Operator 2: is No
		{name: "no match", operator: 2, manga: "No", want: true},
		{name: "no no match Yes", operator: 2, manga: "Yes", want: false},
		// Operator 3: is Unknown
		{name: "unknown match", operator: 3, manga: "Unknown", want: true},
		{name: "unknown empty", operator: 3, manga: "", want: true},
		{name: "unknown no match Yes", operator: 3, manga: "Yes", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &Matcher{Type: MatcherTypeManga, Operator: tt.operator}
			got := matcher.Match(&ComicBook{Manga: tt.manga})
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAllPropertiesMatcher verifies cross-field searching for each option
func TestAllPropertiesMatcher(t *testing.T) {
	book := &ComicBook{
		Series:              "Batman",
		AlternateSeries:     "Dark Knight",
		Title:               "Year One",
		Format:              "Annual",
		SeriesGroup:         "DC Black Label",
		StoryArc:            "Knightfall",
		Writer:              "Frank Miller",
		Penciller:           "David Mazzucchelli",
		Inker:               "Richmond Lewis",
		Colorist:            "Richmond Lewis",
		Letterer:            "Todd Klein",
		CoverArtist:         "Mazzucchelli",
		Editor:              "Denny O'Neil",
		Translator:          "Bob Smith",
		Notes:               "Great issue",
		Summary:             "Batman begins",
		Review:              "Excellent",
		Tags:                "classic, noir",
		Characters:          "Batman, Gordon",
		Teams:               "GCPD",
		MainCharacterOrTeam: "Batman",
		Locations:           "Gotham",
		ScanInformation:     "CBR scan",
		Genre:               "Superhero",
		Publisher:           "DC Comics",
		Imprint:             "DC",
		Number:              "1",
		Volume:              2,
		Year:                1987,
		FilePath:            "/comics/batman/year-one.cbz",
	}

	tests := []struct {
		name       string
		option     AllPropertiesOption
		matchValue string
		want       bool
	}{
		// All option searches everything
		{name: "All finds series", option: AllPropertiesAll, matchValue: "Batman", want: true},
		{name: "All finds writer", option: AllPropertiesAll, matchValue: "Miller", want: true},
		{name: "All finds publisher", option: AllPropertiesAll, matchValue: "DC Comics", want: true},
		{name: "All no match", option: AllPropertiesAll, matchValue: "Marvel", want: false},

		// Series option
		{name: "Series finds series", option: AllPropertiesSeries, matchValue: "Batman", want: true},
		{name: "Series finds storyarc", option: AllPropertiesSeries, matchValue: "Knightfall", want: true},
		{name: "Series not find publisher", option: AllPropertiesSeries, matchValue: "DC Comics", want: false},

		// Writer option
		{name: "Writer finds writer", option: AllPropertiesWriter, matchValue: "Miller", want: true},
		{name: "Writer not find penciller", option: AllPropertiesWriter, matchValue: "Mazzucchelli", want: false},

		// Artists option includes all credits
		{name: "Artists finds writer", option: AllPropertiesArtists, matchValue: "Miller", want: true},
		{name: "Artists finds letterer", option: AllPropertiesArtists, matchValue: "Klein", want: true},
		{name: "Artists not find publisher", option: AllPropertiesArtists, matchValue: "DC Comics", want: false},

		// Descriptive option
		{name: "Descriptive finds notes", option: AllPropertiesDescriptive, matchValue: "Great", want: true},
		{name: "Descriptive finds characters", option: AllPropertiesDescriptive, matchValue: "Gordon", want: true},
		{name: "Descriptive not find writer", option: AllPropertiesDescriptive, matchValue: "Miller", want: false},

		// File option
		{name: "File finds path", option: AllPropertiesFile, matchValue: "year-one", want: true},
		{name: "File not find writer", option: AllPropertiesFile, matchValue: "Miller", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := &Matcher{
				Type:                MatcherTypeAllProperties,
				Operator:            OperatorContains,
				IgnoreCase:          true,
				MatchValue:          tt.matchValue,
				AllPropertiesOption: tt.option,
			}
			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMiscStringMatchers verifies ISBN and Web field extraction
func TestMiscStringMatchers(t *testing.T) {
	book := &ComicBook{ISBN: "978-1-56389-341-5", Web: "https://dc.com"}
	for _, tt := range []struct {
		matcherType MatcherType
		value       string
	}{
		{MatcherTypeISBN, book.ISBN},
		{MatcherTypeWeb, book.Web},
	} {
		t.Run(string(tt.matcherType), func(t *testing.T) {
			m := &Matcher{Type: tt.matcherType, Operator: OperatorEquals, IgnoreCase: true, MatchValue: tt.value}
			if !m.Match(book) {
				t.Errorf("expected match")
			}
			if m.Match(&ComicBook{}) {
				t.Errorf("expected no match for empty book")
			}
		})
	}
}

// TestOwnershipMatchers verifies book ownership/condition string fields
func TestOwnershipMatchers(t *testing.T) {
	book := &ComicBook{
		BookAge:             "Modern",
		BookCondition:       "Near Mint",
		BookStore:           "Mile High",
		BookOwner:           "Patrick",
		BookCollectionStatus: "Read",
		BookNotes:           "Signed copy",
		BookLocation:        "Shelf A",
	}
	for _, tt := range []struct {
		matcherType MatcherType
		value       string
	}{
		{MatcherTypeBookAge, book.BookAge},
		{MatcherTypeBookCondition, book.BookCondition},
		{MatcherTypeBookStore, book.BookStore},
		{MatcherTypeBookOwner, book.BookOwner},
		{MatcherTypeBookCollectionStatus, book.BookCollectionStatus},
		{MatcherTypeBookNotes, book.BookNotes},
		{MatcherTypeBookLocation, book.BookLocation},
	} {
		t.Run(string(tt.matcherType), func(t *testing.T) {
			m := &Matcher{Type: tt.matcherType, Operator: OperatorEquals, IgnoreCase: true, MatchValue: tt.value}
			if !m.Match(book) {
				t.Errorf("expected match")
			}
			if m.Match(&ComicBook{}) {
				t.Errorf("expected no match for empty book")
			}
		})
	}
}

// TestAlternateNumericMatchers verifies AlternateNumber, AlternateCount, FileSize, CommunityRating
func TestAlternateNumericMatchers(t *testing.T) {
	book := &ComicBook{
		AlternateNumber: "2",
		AlternateCount:  5,
		FileSize:        1024 * 1024,
		CommunityRating: 4.5,
	}
	tests := []struct {
		name        string
		matcherType MatcherType
		operator    MatchOperator
		matchVal    string
		want        bool
	}{
		{name: "AlternateNumber equals 2", matcherType: MatcherTypeAlternateNumber, operator: OperatorEquals, matchVal: "2", want: true},
		{name: "AlternateNumber greater 1", matcherType: MatcherTypeAlternateNumber, operator: OperatorGreater, matchVal: "1", want: true},
		{name: "AlternateCount equals 5", matcherType: MatcherTypeAlternateCount, operator: OperatorEquals, matchVal: "5", want: true},
		{name: "AlternateCount lesser 10", matcherType: MatcherTypeAlternateCount, operator: OperatorLesser, matchVal: "10", want: true},
		{name: "FileSize greater 0", matcherType: MatcherTypeFileSize, operator: OperatorGreater, matchVal: "0", want: true},
		{name: "FileSize equals 1048576", matcherType: MatcherTypeFileSize, operator: OperatorEquals, matchVal: "1048576", want: true},
		{name: "CommunityRating greater 4", matcherType: MatcherTypeCommunityRating, operator: OperatorGreater, matchVal: "4", want: true},
		{name: "CommunityRating equals 4.5", matcherType: MatcherTypeCommunityRating, operator: OperatorEquals, matchVal: "4.5", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Matcher{Type: tt.matcherType, Operator: tt.operator, MatchValue: tt.matchVal}
			if m.Match(book) != tt.want {
				t.Errorf("Match() = %v, want %v", !tt.want, tt.want)
			}
		})
	}
}

// TestFileDateMatchers verifies Modified and Creation date matchers
func TestFileDateMatchers(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	book := &ComicBook{
		FileModifiedTime: ComicTime{Time: now},
		FileCreationTime: ComicTime{Time: now.AddDate(-1, 0, 0)},
	}
	tests := []struct {
		name        string
		matcherType MatcherType
		operator    MatchOperator
		matchVal    string
		want        bool
	}{
		{name: "Modified after yesterday", matcherType: MatcherTypeModified, operator: OperatorIsAfter, matchVal: now.AddDate(0, 0, -1).Format(time.RFC3339), want: true},
		{name: "Modified in last 1 day", matcherType: MatcherTypeModified, operator: OperatorIsInLastDays, matchVal: "1", want: true},
		{name: "Creation before now", matcherType: MatcherTypeCreation, operator: OperatorIsBefore, matchVal: now.Format(time.RFC3339), want: true},
		{name: "Modified zero book no match", matcherType: MatcherTypeModified, operator: OperatorIsAfter, matchVal: "2000-01-01T00:00:00Z", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := book
			if tt.name == "Modified zero book no match" {
				b = &ComicBook{}
			}
			m := &Matcher{Type: tt.matcherType, Operator: tt.operator, MatchValue: tt.matchVal}
			if m.Match(b) != tt.want {
				t.Errorf("Match() = %v, want %v", !tt.want, tt.want)
			}
		})
	}
}

// TestPublishedReleasedDateMatchers verifies Published (computed) and Released date matchers
func TestPublishedReleasedDateMatchers(t *testing.T) {
	released := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	book := &ComicBook{
		Year:         2020,
		Month:        3,
		Day:          15,
		ReleasedTime: ComicTime{Time: released},
	}
	tests := []struct {
		name        string
		matcherType MatcherType
		operator    MatchOperator
		matchVal    string
		want        bool
	}{
		// Published = 2020-03-15
		{name: "Published after 2019", matcherType: MatcherTypePublished, operator: OperatorIsAfter, matchVal: "2019-01-01T00:00:00Z", want: true},
		{name: "Published before 2021", matcherType: MatcherTypePublished, operator: OperatorIsBefore, matchVal: "2021-01-01T00:00:00Z", want: true},
		{name: "Published not after 2021", matcherType: MatcherTypePublished, operator: OperatorIsAfter, matchVal: "2021-01-01T00:00:00Z", want: false},
		{name: "Published zero year no match", matcherType: MatcherTypePublished, operator: OperatorIsAfter, matchVal: "2000-01-01T00:00:00Z", want: false},
		// Released = 2023-06-15
		{name: "Released after 2022", matcherType: MatcherTypeReleased, operator: OperatorIsAfter, matchVal: "2022-01-01T00:00:00Z", want: true},
		{name: "Released before 2024", matcherType: MatcherTypeReleased, operator: OperatorIsBefore, matchVal: "2024-01-01T00:00:00Z", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := book
			if tt.name == "Published zero year no match" {
				b = &ComicBook{Month: 3, Day: 15}
			}
			m := &Matcher{Type: tt.matcherType, Operator: tt.operator, MatchValue: tt.matchVal}
			if m.Match(b) != tt.want {
				t.Errorf("Match() = %v, want %v", !tt.want, tt.want)
			}
		})
	}
}

// TestP4NumericMatchers verifies NewPages, BookmarkCount, BookPrice, Day, Week
func TestP4NumericMatchers(t *testing.T) {
	book := &ComicBook{
		Year:      2020,
		Month:     3,
		Day:       15,
		NewPages:  7,
		BookPrice: 4.99,
		Pages: []ComicPageInfo{
			{Image: 0, Bookmark: "Chapter 1"},
			{Image: 1},
			{Image: 2, Bookmark: "Plot twist"},
		},
	}
	_, isoWeek := time.Date(2020, 3, 15, 0, 0, 0, 0, time.UTC).ISOWeek()

	tests := []struct {
		name        string
		matcherType MatcherType
		operator    MatchOperator
		matchVal    string
		want        bool
	}{
		{name: "Day equals 15", matcherType: MatcherTypeDay, operator: OperatorEquals, matchVal: "15", want: true},
		{name: "Day greater 10", matcherType: MatcherTypeDay, operator: OperatorGreater, matchVal: "10", want: true},
		{name: "Day lesser 20", matcherType: MatcherTypeDay, operator: OperatorLesser, matchVal: "20", want: true},
		{name: "Week equals iso", matcherType: MatcherTypeWeek, operator: OperatorEquals, matchVal: strconv.Itoa(isoWeek), want: true},
		{name: "NewPages equals 7", matcherType: MatcherTypeNewPages, operator: OperatorEquals, matchVal: "7", want: true},
		{name: "NewPages greater 5", matcherType: MatcherTypeNewPages, operator: OperatorGreater, matchVal: "5", want: true},
		{name: "BookmarkCount equals 2", matcherType: MatcherTypeBookmarkCount, operator: OperatorEquals, matchVal: "2", want: true},
		{name: "BookmarkCount greater 1", matcherType: MatcherTypeBookmarkCount, operator: OperatorGreater, matchVal: "1", want: true},
		{name: "BookPrice greater 4", matcherType: MatcherTypeBookPrice, operator: OperatorGreater, matchVal: "4", want: true},
		{name: "BookPrice lesser 10", matcherType: MatcherTypeBookPrice, operator: OperatorLesser, matchVal: "10", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Matcher{Type: tt.matcherType, Operator: tt.operator, MatchValue: tt.matchVal}
			if m.Match(book) != tt.want {
				t.Errorf("Match() = %v, want %v", !tt.want, tt.want)
			}
		})
	}

	// Week returns -1 for books with no year
	emptyBook := &ComicBook{}
	mWeek := &Matcher{Type: MatcherTypeWeek, Operator: OperatorEquals, MatchValue: "-1"}
	if !mWeek.Match(emptyBook) {
		t.Error("Week matcher should return -1 for books with no year")
	}

	// BookPrice -1 (unset) should not match positive values
	unpricedBook := &ComicBook{BookPrice: -1}
	mPrice := &Matcher{Type: MatcherTypeBookPrice, Operator: OperatorGreater, MatchValue: "0"}
	if mPrice.Match(unpricedBook) {
		t.Error("unset BookPrice (-1) should not match")
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

// TestCustomValuesMatcher tests the CustomValues matcher.
// ComicRack's GetCustomValue returns null for absent keys; null is coerced to ""
// before comparison. So "key is blank" (MatchValue2="") is true when the key is
// absent OR has an empty value. NOT("key is blank") = key has a non-empty value.
func TestCustomValuesMatcher(t *testing.T) {
	tests := []struct {
		name     string
		store    string
		matchKey string
		matchVal string
		not      bool
		want     bool
	}{
		// "is blank" (MatchValue2=""): true when key absent or has empty value
		{
			name:     "key absent - is blank = true",
			store:    ",comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			want:     true,
		},
		{
			name:     "key with value - is blank = false",
			store:    ",comicvine_issue=133658,comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			want:     false,
		},
		{
			name:     "empty store - is blank = true",
			store:    "",
			matchKey: "comicvine_issue",
			matchVal: "",
			want:     true,
		},
		// NOT("is blank") = key has a non-empty value
		{
			name:     "NOT blank - passes book with value (scraped)",
			store:    ",comicvine_issue=133658,comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			not:      true,
			want:     true,
		},
		{
			name:     "NOT blank - rejects book with absent key (unscraped)",
			store:    ",comicvine_volume=20784",
			matchKey: "comicvine_issue",
			matchVal: "",
			not:      true,
			want:     false,
		},
		{
			name:     "NOT blank - rejects empty store",
			store:    "",
			matchKey: "comicvine_issue",
			matchVal: "",
			not:      true,
			want:     false,
		},
		// Specific value (MatchValue2 non-empty)
		{
			name:     "key+value exact match",
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

// TestCountMatcherNumeric verifies Count uses numeric matching so "Count NOT empty"
// correctly distinguishes unset (0) from set (>0) books.
func TestCountMatcherNumeric(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		matchVal  string
		not       bool
		want      bool
	}{
		{
			name: "count empty (0) matches empty check",
			count: 0, matchVal: "", want: true,
		},
		{
			name: "count set does not match empty check",
			count: 6, matchVal: "", want: false,
		},
		{
			name: "NOT count empty passes when count is set",
			count: 6, matchVal: "", not: true, want: true,
		},
		{
			name: "NOT count empty fails when count is unset",
			count: 0, matchVal: "", not: true, want: false,
		},
		{
			name: "count equals specific value",
			count: 6, matchVal: "6", want: true,
		},
		{
			name: "count not equal to specific value",
			count: 4, matchVal: "6", want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			book := &ComicBook{Count: tt.count}
			xmlM := &ComicBookMatcher{
				Type:       "ComicBookCountMatcher",
				MatchValue: tt.matchVal,
				Not:        tt.not,
			}
			matcher, err := NewMatcherFromXML(xmlM)
			if err != nil {
				t.Fatalf("NewMatcherFromXML error: %v", err)
			}
			got := matcher.Match(book)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v (count=%d, matchVal=%q, not=%v)",
					got, tt.want, tt.count, tt.matchVal, tt.not)
			}
		})
	}
}

// seriesList builds a ComicListItem with a single series matcher for testing.
func seriesList(xmlType, matchOp, matchVal string, not bool) *ComicListItem {
	return &ComicListItem{
		Type:        "ComicSmartListItem",
		MatcherMode: "And",
		Matchers: []ComicBookMatcher{
			{Type: xmlType, MatchOperator: matchOp, MatchValue: matchVal, Not: not},
		},
	}
}

// evalSeries runs matchBooks and returns which of books matched.
func evalSeries(list *ComicListItem, books []*ComicBook) []*ComicBook {
	return matchBooks(list, books)
}

func TestSeriesCountMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X-Men", Volume: 1, Number: "1"},
		{ID: "2", Series: "X-Men", Volume: 1, Number: "2"},
		{ID: "3", Series: "X-Men", Volume: 1, Number: "3"},
		{ID: "4", Series: "Batman", Volume: 1, Number: "1"},
	}
	// Matches books whose series has exactly 3 books
	list := seriesList("SmartListSeriesCountMatcher", "0", "3", false) // op 0 = Equals
	got := evalSeries(list, books)
	if len(got) != 3 {
		t.Errorf("got %d matches, want 3 (all X-Men books)", len(got))
	}
	for _, b := range got {
		if b.Series != "X-Men" {
			t.Errorf("unexpected match: %s", b.Series)
		}
	}
}

func TestSeriesAverageRatingMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, Rating: 4.0},
		{ID: "2", Series: "X", Volume: 1, Rating: 2.0},
		{ID: "3", Series: "Y", Volume: 1, Rating: 1.0},
	}
	// Series average rating for X = 3.0; match books in series with avg rating > 2
	list := seriesList("SmartListSeriesAverageRatingMatcher", "1", "2", false) // op 1 = Greater
	got := evalSeries(list, books)
	if len(got) != 2 {
		t.Errorf("got %d matches, want 2 (X books)", len(got))
	}
}

func TestSeriesMinMaxYearMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, Year: 1990},
		{ID: "2", Series: "X", Volume: 1, Year: 2005},
		{ID: "3", Series: "Y", Volume: 1, Year: 2000},
	}
	// MinYear for X is 1990; match series where min year <= 1990
	list := seriesList("SmartListSeriesMinYearMatcher", "2", "1990", false) // Greater or Equals... use op=0 (equals)
	list.Matchers[0].MatchOperator = "0" // Equals
	got := evalSeries(list, books)
	if len(got) != 2 {
		t.Errorf("got %d matches, want 2 (both X books)", len(got))
	}

	// MaxYear for X is 2005
	listMax := seriesList("SmartListSeriesMaxYearMatcher", "0", "2005", false)
	got2 := evalSeries(listMax, books)
	if len(got2) != 2 {
		t.Errorf("got %d max-year matches, want 2", len(got2))
	}
}

func TestSeriesFirstLastNumberMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, Number: "3"},
		{ID: "2", Series: "X", Volume: 1, Number: "1"},
		{ID: "3", Series: "X", Volume: 1, Number: "7"},
	}
	// FirstNumber = 1
	listFirst := seriesList("SmartListSeriesMinNumbertMatcher", "0", "1", false) // XmlType alias
	got := evalSeries(listFirst, books)
	if len(got) != 3 {
		t.Errorf("FirstNumber: got %d matches, want 3", len(got))
	}

	// LastNumber = 7
	listLast := seriesList("SmartListSeriesMaxNumbertMatcher", "0", "7", false) // XmlType alias
	got2 := evalSeries(listLast, books)
	if len(got2) != 3 {
		t.Errorf("LastNumber: got %d matches, want 3", len(got2))
	}
}

func TestSeriesGapMatchers(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, Number: "1"},
		{ID: "2", Series: "X", Volume: 1, Number: "2"},
		{ID: "3", Series: "X", Volume: 1, Number: "5"}, // gap after #2
		{ID: "4", Series: "Y", Volume: 1, Number: "1"},
		{ID: "5", Series: "Y", Volume: 1, Number: "2"},
	}
	// Gaps: X has 1 gap, Y has 0 gaps
	listGaps := seriesList("SmartListSeriesGapsMatcher", "1", "0", false) // op 1 = Greater
	got := evalSeries(listGaps, books)
	if len(got) != 3 {
		t.Errorf("Gaps>0: got %d matches, want 3 (X books)", len(got))
	}

	// EndOfGap: book #5 in X is the end of a gap
	listEndGap := seriesList("SmartListSeriesEndOfGapMatcher", "0", "Yes", false)
	gotEnd := evalSeries(listEndGap, books)
	if len(gotEnd) != 1 || gotEnd[0].ID != "3" {
		t.Errorf("EndOfGap: expected only book#3, got %v", gotEnd)
	}

	// StartOfGap: book #2 in X is the start of a gap
	listStartGap := seriesList("SmartListSeriesStartOfGapMatcher", "0", "Yes", false)
	gotStart := evalSeries(listStartGap, books)
	if len(gotStart) != 1 || gotStart[0].ID != "2" {
		t.Errorf("StartOfGap: expected only book#2, got %v", gotStart)
	}
}

func TestSeriesAllCompleteMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, SeriesComplete: "Yes"},
		{ID: "2", Series: "X", Volume: 1, SeriesComplete: "Yes"},
		{ID: "3", Series: "Y", Volume: 1, SeriesComplete: "No"},
	}
	listYes := seriesList("SmartListSeriesAllCompleteMatcher", "0", "Yes", false)
	got := evalSeries(listYes, books)
	if len(got) != 2 {
		t.Errorf("AllComplete=Yes: got %d, want 2", len(got))
	}
}

func TestSeriesPageCountMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, PageCount: 22},
		{ID: "2", Series: "X", Volume: 1, PageCount: 22},
		{ID: "3", Series: "Y", Volume: 1, PageCount: 10},
	}
	// Series X total page count = 44
	listPC := seriesList("SmartListSeriesPageCountMatcher", "1", "40", false) // op 1 = Greater
	got := evalSeries(listPC, books)
	if len(got) != 2 {
		t.Errorf("PageCount>40: got %d, want 2 (X books)", len(got))
	}
}

func TestSeriesPercentReadMatcher(t *testing.T) {
	books := []*ComicBook{
		// read
		{ID: "1", Series: "X", Volume: 1, PageCount: 22, LastPageRead: 21, OpenCount: 1},
		// unread
		{ID: "2", Series: "X", Volume: 1, PageCount: 22, LastPageRead: 0, OpenCount: 0},
		{ID: "3", Series: "Y", Volume: 1, PageCount: 22, LastPageRead: 21, OpenCount: 1},
	}
	// X: 1/2 read = 50%; Y: 1/1 = 100%
	listPct := seriesList("SmartListSeriesPercentReadMatcher", "0", "50", false) // Equals 50
	got := evalSeries(listPct, books)
	if len(got) != 2 {
		t.Errorf("PercentRead=50: got %d, want 2 (X books)", len(got))
	}
}

func TestSeriesRunningTimeYearsMatcher(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1, Year: 1990},
		{ID: "2", Series: "X", Volume: 1, Year: 2000},
		{ID: "3", Series: "Y", Volume: 1, Year: 2020},
	}
	// X: 2000-1990=10; Y: 0
	listRTY := seriesList("SmartListSeriesRunningTimeYearsMatcher", "0", "10", false)
	got := evalSeries(listRTY, books)
	if len(got) != 2 {
		t.Errorf("RunningTimeYears=10: got %d, want 2 (X books)", len(got))
	}
}

func TestSeriesNegation(t *testing.T) {
	books := []*ComicBook{
		{ID: "1", Series: "X", Volume: 1},
		{ID: "2", Series: "X", Volume: 1},
		{ID: "3", Series: "Y", Volume: 1},
	}
	// NOT SeriesCount = 2 → should match Y (1 book) and NOT X (2 books)
	list := seriesList("SmartListSeriesCountMatcher", "0", "2", true)
	got := evalSeries(list, books)
	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("NOT SeriesCount=2: got %d matches, want 1 (Y book)", len(got))
	}
}

func TestSeriesMatcherMissingStatsReturnsFalse(t *testing.T) {
	book := &ComicBook{Series: "X", Volume: 1}
	xmlM := &ComicBookMatcher{Type: "SmartListSeriesCountMatcher", MatchValue: "1"}
	// Pass nil stats explicitly
	got := evaluateSeriesMatcher(xmlM, book, nil)
	if got {
		t.Error("expected false when stats is nil")
	}
}
