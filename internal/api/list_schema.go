package api

import (
	"encoding/json"
	"net/http"
)

// MatcherTypeInfo describes a single matcher type for the UI.
type MatcherTypeInfo struct {
	ID        string `json:"id"`        // Full XML type, e.g. "ComicBookSeriesMatcher"
	Label     string `json:"label"`     // Human-readable label
	Category  string `json:"category"`  // Grouping label
	FieldType string `json:"fieldType"` // string | numeric | date | yesno | manga | all_properties
}

// OperatorInfo describes one operator choice.
type OperatorInfo struct {
	Value     string `json:"value"`               // Numeric string sent in MatchOperator field
	Label     string `json:"label"`               // Human-readable label
	HasValue  bool   `json:"hasValue"`            // Whether a primary value input is needed
	HasValue2 bool   `json:"hasValue2,omitempty"` // Whether a second value is needed (range)
}

// ListSchema is the static metadata returned by GET /api/library/lists/schema.
type ListSchema struct {
	MatcherTypes []MatcherTypeInfo               `json:"matcherTypes"`
	Operators    map[string][]OperatorInfo       `json:"operators"`
	MatcherModes []struct{ Value, Label string } `json:"matcherModes"`
}

var listSchemaCache *ListSchema

func getListSchema() *ListSchema {
	if listSchemaCache != nil {
		return listSchemaCache
	}

	listSchemaCache = &ListSchema{
		MatcherModes: []struct{ Value, Label string }{
			{"And", "Match ALL conditions (AND)"},
			{"Or", "Match ANY condition (OR)"},
		},
		Operators: map[string][]OperatorInfo{
			"string": {
				{"0", "equals", true, false},
				{"1", "contains", true, false},
				{"2", "contains any of", true, false},
				{"3", "contains all of", true, false},
				{"4", "starts with", true, false},
				{"5", "ends with", true, false},
				{"7", "matches regex", true, false},
			},
			"numeric": {
				{"0", "equals", true, false},
				{"1", "greater than", true, false},
				{"2", "less than", true, false},
				{"3", "in range", true, true},
			},
			"date": {
				{"0", "equals", true, false},
				{"1", "is after", true, false},
				{"2", "is before", true, false},
				{"3", "is in last N days", true, false},
				{"4", "is in date range", true, true},
			},
			"yesno": {
				{"0", "Yes", false, false},
				{"1", "No", false, false},
				{"2", "Unknown", false, false},
			},
			"manga": {
				{"0", "Yes or Yes (Right to Left)", false, false},
				{"1", "Yes (Right to Left)", false, false},
				{"2", "No", false, false},
				{"3", "Unknown", false, false},
			},
		},
		MatcherTypes: []MatcherTypeInfo{
			// Title & Series
			{ID: "ComicBookSeriesMatcher", Label: "Series", Category: "Title & Series", FieldType: "string"},
			{ID: "ComicBookAlternateSeriesMatcher", Label: "Alternate Series", Category: "Title & Series", FieldType: "string"},
			{ID: "ComicBookSeriesGroupMatcher", Label: "Series Group", Category: "Title & Series", FieldType: "string"},
			{ID: "ComicBookTitleMatcher", Label: "Title", Category: "Title & Series", FieldType: "string"},
			{ID: "ComicBookNumberMatcher", Label: "Number", Category: "Title & Series", FieldType: "string"},
			{ID: "ComicBookCountMatcher", Label: "Count", Category: "Title & Series", FieldType: "numeric"},
			{ID: "ComicBookAlternateNumberMatcher", Label: "Alternate Number", Category: "Title & Series", FieldType: "numeric"},
			{ID: "ComicBookAlternateCountMatcher", Label: "Alternate Count", Category: "Title & Series", FieldType: "numeric"},
			{ID: "ComicBookVolumeMatcher", Label: "Volume", Category: "Title & Series", FieldType: "numeric"},
			{ID: "ComicBookStoryArcMatcher", Label: "Story Arc", Category: "Title & Series", FieldType: "string"},
			// Publishing
			{ID: "ComicBookPublisherMatcher", Label: "Publisher", Category: "Publishing", FieldType: "string"},
			{ID: "ComicBookImprintMatcher", Label: "Imprint", Category: "Publishing", FieldType: "string"},
			{ID: "ComicBookYearMatcher", Label: "Year", Category: "Publishing", FieldType: "numeric"},
			{ID: "ComicBookMonthMatcher", Label: "Month", Category: "Publishing", FieldType: "numeric"},
			{ID: "ComicBookDayMatcher", Label: "Day", Category: "Publishing", FieldType: "numeric"},
			{ID: "ComicBookWeekMatcher", Label: "Week", Category: "Publishing", FieldType: "numeric"},
			{ID: "ComicBookPublishedMatcher", Label: "Published Date", Category: "Publishing", FieldType: "date"},
			{ID: "ComicBookReleasedMatcher", Label: "Released Date", Category: "Publishing", FieldType: "date"},
			{ID: "ComicBookFormatMatcher", Label: "Format", Category: "Publishing", FieldType: "string"},
			{ID: "ComicBookLanguageMatcher", Label: "Language", Category: "Publishing", FieldType: "string"},
			{ID: "ComicBookWebMatcher", Label: "Web URL", Category: "Publishing", FieldType: "string"},
			{ID: "ComicBookISBNMatcher", Label: "ISBN", Category: "Publishing", FieldType: "string"},
			// Content
			{ID: "ComicBookGenreMatcher", Label: "Genre", Category: "Content", FieldType: "string"},
			{ID: "ComicBookAgeRatingMatcher", Label: "Age Rating", Category: "Content", FieldType: "string"},
			{ID: "ComicBookTagsMatcher", Label: "Tags", Category: "Content", FieldType: "string"},
			{ID: "ComicBookNotesMatcher", Label: "Notes", Category: "Content", FieldType: "string"},
			{ID: "ComicBookSummaryMatcher", Label: "Summary", Category: "Content", FieldType: "string"},
			{ID: "ComicBookReviewMatcher", Label: "Review", Category: "Content", FieldType: "string"},
			{ID: "ComicBookScanInformationMatcher", Label: "Scan Information", Category: "Content", FieldType: "string"},
			{ID: "ComicBookPageCountMatcher", Label: "Page Count", Category: "Content", FieldType: "numeric"},
			{ID: "ComicBookBlackAndWhiteMatcher", Label: "Black & White", Category: "Content", FieldType: "yesno"},
			{ID: "ComicBookMangaMatcher", Label: "Manga", Category: "Content", FieldType: "manga"},
			// Characters & Teams
			{ID: "ComicBookCharactersMatcher", Label: "Characters", Category: "Characters & Teams", FieldType: "string"},
			{ID: "ComicBookTeamsMatcher", Label: "Teams", Category: "Characters & Teams", FieldType: "string"},
			{ID: "ComicBookMainCharacterOrTeamMatcher", Label: "Main Character/Team", Category: "Characters & Teams", FieldType: "string"},
			{ID: "ComicBookLocationsMatcher", Label: "Locations", Category: "Characters & Teams", FieldType: "string"},
			// Credits
			{ID: "ComicBookWriterMatcher", Label: "Writer", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookPencillerMatcher", Label: "Penciller", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookInkerMatcher", Label: "Inker", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookColoristMatcher", Label: "Colorist", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookLettererMatcher", Label: "Letterer", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookCoverArtistMatcher", Label: "Cover Artist", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookEditorMatcher", Label: "Editor", Category: "Credits", FieldType: "string"},
			{ID: "ComicBookTranslatorMatcher", Label: "Translator", Category: "Credits", FieldType: "string"},
			// Reading State
			{ID: "ComicBookReadPercentageMatcher", Label: "Read %", Category: "Reading State", FieldType: "numeric"},
			{ID: "ComicBookRatingMatcher", Label: "Rating", Category: "Reading State", FieldType: "numeric"},
			{ID: "ComicBookCommunityRatingMatcher", Label: "Community Rating", Category: "Reading State", FieldType: "numeric"},
			{ID: "ComicBookCheckedMatcher", Label: "Checked", Category: "Reading State", FieldType: "yesno"},
			{ID: "ComicBookAddedMatcher", Label: "Date Added", Category: "Reading State", FieldType: "date"},
			{ID: "ComicBookOpenedMatcher", Label: "Date Opened", Category: "Reading State", FieldType: "date"},
			{ID: "ComicBookNewPagesMatcher", Label: "New Pages", Category: "Reading State", FieldType: "numeric"},
			{ID: "ComicBookBookmarkCountMatcher", Label: "Bookmark Count", Category: "Reading State", FieldType: "numeric"},
			// File
			{ID: "ComicBookFileMatcher", Label: "File Name", Category: "File", FieldType: "string"},
			{ID: "ComicBookDirectoryMatcher", Label: "Directory", Category: "File", FieldType: "string"},
			{ID: "ComicBookFullPathMatcher", Label: "Full Path", Category: "File", FieldType: "string"},
			{ID: "ComicBookFileFormatMatcher", Label: "File Format", Category: "File", FieldType: "string"},
			{ID: "ComicBookFileSizeMatcher", Label: "File Size", Category: "File", FieldType: "numeric"},
			{ID: "ComicBookModifiedMatcher", Label: "Date Modified", Category: "File", FieldType: "date"},
			{ID: "ComicBookCreationMatcher", Label: "Date Created", Category: "File", FieldType: "date"},
			{ID: "ComicBookIsMissingMatcher", Label: "File Missing", Category: "File", FieldType: "yesno"},
			{ID: "ComicBookIsLinkedMatcher", Label: "Linked", Category: "File", FieldType: "yesno"},
			// Catalog
			{ID: "ComicBookBookAgeMatcher", Label: "Book Age", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookConditionMatcher", Label: "Condition", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookStoreMatcher", Label: "Store", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookOwnerMatcher", Label: "Owner", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookCollectionStatusMatcher", Label: "Collection Status", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookNotesMatcher", Label: "Book Notes", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookLocationMatcher", Label: "Location", Category: "Catalog", FieldType: "string"},
			{ID: "ComicBookBookPriceMatcher", Label: "Price", Category: "Catalog", FieldType: "numeric"},
		},
	}
	return listSchemaCache
}

// handleGetListSchema returns static matcher type and operator metadata for the editor UI.
func (s *Server) handleGetListSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getListSchema())
}
