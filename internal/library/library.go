package library

import (
	"encoding/xml"
	"os"
)

// ComicLibrary represents the root library container from ComicDb.xml
type ComicLibrary struct {
	XMLName    xml.Name        `xml:"ComicDatabase"`
	ID         string          `xml:"Id,attr"`
	Name       string          `xml:"Name,omitempty"`
	Books      []ComicBook     `xml:"Books>Book"`
	ComicLists []ComicListItem `xml:"ComicLists>Item"`
}

// ComicBook represents an individual comic entry in the library
type ComicBook struct {
	// Identification
	ID       string `xml:"Id,attr"`             // Id attribute
	FilePath string `xml:"File,attr,omitempty"` // File attribute for file path

	// Title & Series
	Title           string `xml:"Title,omitempty"`
	Series          string `xml:"Series,omitempty"`
	Number          string `xml:"Number,omitempty"`
	Count           int    `xml:"Count,omitempty"`
	Volume          int    `xml:"Volume,omitempty"`
	AlternateSeries string `xml:"AlternateSeries,omitempty"`
	AlternateNumber string `xml:"AlternateNumber,omitempty"`
	AlternateCount  int    `xml:"AlternateCount,omitempty"`
	StoryArc        string `xml:"StoryArc,omitempty"`
	SeriesGroup     string `xml:"SeriesGroup,omitempty"`

	// Content Metadata
	Summary             string `xml:"Summary,omitempty"`
	Notes               string `xml:"Notes,omitempty"`
	Review              string `xml:"Review,omitempty"`
	Tags                string `xml:"Tags,omitempty"`
	Characters          string `xml:"Characters,omitempty"`
	Teams               string `xml:"Teams,omitempty"`
	MainCharacterOrTeam string `xml:"MainCharacterOrTeam,omitempty"`
	Locations           string `xml:"Locations,omitempty"`

	// Publishing Information
	Year        int    `xml:"Year,omitempty"`
	Month       int    `xml:"Month,omitempty"`
	Day         int    `xml:"Day,omitempty"`
	Publisher   string `xml:"Publisher,omitempty"`
	Imprint     string `xml:"Imprint,omitempty"`
	Format      string `xml:"Format,omitempty"`
	LanguageISO string `xml:"LanguageISO,omitempty"`
	Genre       string `xml:"Genre,omitempty"`
	Web         string `xml:"Web,omitempty"`
	AgeRating   string `xml:"AgeRating,omitempty"`

	// Creative Credits
	Writer      string `xml:"Writer,omitempty"`
	Penciller   string `xml:"Penciller,omitempty"`
	Inker       string `xml:"Inker,omitempty"`
	Colorist    string `xml:"Colorist,omitempty"`
	Letterer    string `xml:"Letterer,omitempty"`
	CoverArtist string `xml:"CoverArtist,omitempty"`
	Editor      string `xml:"Editor,omitempty"`
	Translator  string `xml:"Translator,omitempty"`

	// Physical Properties
	PageCount           int    `xml:"PageCount,omitempty"`
	BlackAndWhite       string `xml:"BlackAndWhite,omitempty"` // "Unknown", "Yes", "No"
	Manga               string `xml:"Manga,omitempty"`         // "Unknown", "Yes", "YesRightToLeft"
	PreferredFrontCover int    `xml:"PreferredFrontCover,omitempty"`

	// User Tracking
	Rating               float64   `xml:"Rating,omitempty"`
	CommunityRating      float64   `xml:"CommunityRating,omitempty"`
	CurrentPage          int       `xml:"CurrentPage,omitempty"`
	LastPage             int       `xml:"LastPage,omitempty"`
	OpenedTime           ComicTime `xml:"Opened,omitempty"`
	OpenCount            int       `xml:"OpenCount,omitempty"`
	AddedTime            ComicTime `xml:"Added,omitempty"`
	ReleasedTime         ComicTime `xml:"Released,omitempty"`
	LastPageRead         int       `xml:"LastPageRead,omitempty"`
	LastOpenedFromListID string    `xml:"LastOpenedFromListId,omitempty"`

	// File System Metadata
	FileSize         int64     `xml:"FileSize,omitempty"`
	FileModifiedTime ComicTime `xml:"FileModifiedTime,omitempty"`
	FileCreationTime ComicTime `xml:"FileCreationTime,omitempty"`

	// Catalog / Ownership
	ISBN                string `xml:"ISBN,omitempty"`
	BookAge             string `xml:"BookAge,omitempty"`
	BookCondition       string `xml:"BookCondition,omitempty"`
	BookStore           string `xml:"BookStore,omitempty"`
	BookOwner           string `xml:"BookOwner,omitempty"`
	BookCollectionStatus string `xml:"BookCollectionStatus,omitempty"`
	BookNotes           string `xml:"BookNotes,omitempty"`
	BookLocation        string `xml:"BookLocation,omitempty"`

	// System Flags
	Checked             bool   `xml:"Checked,attr,omitempty"` // Book attribute
	FileIsMissing       bool   `xml:"Missing,omitempty"`      // File cannot be found
	ComicInfoIsDirty    bool   `xml:"ComicInfoIsDirty,omitempty"`
	SeriesComplete      string `xml:"SeriesComplete,omitempty"` // "Unknown", "Yes", "No"
	EnableProposed      bool   `xml:"EnableProposed,omitempty"`
	EnableDynamicUpdate bool   `xml:"EnableDynamicUpdate,omitempty"`

	// Pages Information
	Pages           []ComicPageInfo `xml:"Pages>Page,omitempty"`
	ScanInformation string          `xml:"ScanInformation,omitempty"`

	// Custom Values
	CustomValuesStore string `xml:"CustomValuesStore,omitempty"` // Comma-separated key=value pairs
}

// Page type constants as defined by ComicRack
const (
	PageTypeFrontCover    = "FrontCover"
	PageTypeBackCover     = "BackCover"
	PageTypeStory         = "Story"
	PageTypeAdvertisement = "Advertisement"
	PageTypeEditorial     = "Editorial"
	PageTypeRoundUp       = "RoundUp"
	PageTypeOther         = "Other"
	PageTypeDeleted       = "Deleted"
)

// ComicPageInfo represents page-level metadata
type ComicPageInfo struct {
	Image       int    `xml:"Image,attr"`
	Type        string `xml:"Type,attr,omitempty"` // Use PageType* constants
	Bookmark    string `xml:"Bookmark,attr,omitempty"`
	ImageSize   int64  `xml:"ImageSize,attr,omitempty"`
	ImageWidth  int    `xml:"ImageWidth,attr,omitempty"`
	ImageHeight int    `xml:"ImageHeight,attr,omitempty"`
	Key         string `xml:"Key,attr,omitempty"`
}

// ComicListItem represents a reading list or smart list
// This is polymorphic in the C# code, but we'll parse common fields
type ComicListItem struct {
	Type        string `xml:"type,attr"`        // xsi:type attribute
	ID          string `xml:"Id,attr"`          // Id attribute
	Name        string `xml:"Name,attr"`        // Name attribute
	MatcherMode string `xml:"MatcherMode,attr"` // "And", "Or" - attribute for smart lists
	Description string `xml:"Description,omitempty"`
	Favorite    bool   `xml:"Favorite,omitempty"`
	BookCount   int    `xml:"BookCount,omitempty"`

	// BaseListId scopes the smart list to match only within another list's result set.
	// Corresponds to the "Match on [source]" dropdown in ComicRack CE.
	BaseListId string `xml:"BaseListId,omitempty"`

	// For ComicReadingList
	Items []ComicReadingListItem `xml:"Books>Book,omitempty"`

	// For ComicSmartListItem
	Matchers []ComicBookMatcher `xml:"Matchers>ComicBookMatcher,omitempty"`

	// For ComicIdListItem — explicit set of books stored by GUID
	BookIds []string `xml:"BookIds>guid,omitempty"`

	// For ComicListItemFolder
	ChildItems []ComicListItem `xml:"Items>Item,omitempty"`
	Collapsed  bool            `xml:"Collapsed,omitempty"`
}

// ComicReadingListItem represents a book reference in a reading list
type ComicReadingListItem struct {
	Series   string `xml:"Series,attr,omitempty"`
	Number   string `xml:"Number,attr,omitempty"`
	Volume   int    `xml:"Volume,attr,omitempty"`
	Year     int    `xml:"Year,attr,omitempty"`
	Format   string `xml:"Format,attr,omitempty"`
	ID       string `xml:"Id,attr,omitempty"`
	FileName string `xml:"FileName,omitempty"`
}

// ComicBookMatcher represents a filter rule for smart lists
// This can be either a value matcher (has MatchOperator/MatchValue) or a group matcher (has nested Matchers)
type ComicBookMatcher struct {
	Type          string             `xml:"type,attr"`                           // xsi:type attribute (e.g., "ComicBookDirectoryMatcher" or "ComicBookGroupMatcher")
	Not           bool               `xml:"Not,attr,omitempty"`                  // Negation flag
	MatchOperator string             `xml:"MatchOperator,attr,omitempty"`        // Operator number (0-7) - only for value matchers
	MatchValue    string             `xml:"MatchValue,omitempty"`                // Value to match (child element) - only for value matchers
	MatchValue2   string             `xml:"MatchValue2,omitempty"`               // Second value for multi-value matchers (Tags, CustomValues)
	Option        string             `xml:"Option,attr,omitempty"`               // Option for AllProperties matcher: "All", "Series", "Writer", "Artists", "Descriptive", "File", "Catalog"
	MatcherMode   string             `xml:"MatcherMode,attr,omitempty"`          // "And" or "Or" - only for group matchers
	Matchers      []ComicBookMatcher `xml:"Matchers>ComicBookMatcher,omitempty"` // Nested matchers - only for group matchers
}

// LoadLibrary loads and parses a ComicRack library XML file
func LoadLibrary(path string) (*ComicLibrary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Check if file is BZIP2 compressed (first byte = 0x01)
	// For now, we'll assume uncompressed XML
	// TODO: Add BZIP2 decompression support if needed

	var library ComicLibrary
	if err := xml.Unmarshal(data, &library); err != nil {
		return nil, err
	}

	return &library, nil
}

// SaveLibrary writes the library back to disk in XML format
func SaveLibrary(path string, library *ComicLibrary) error {
	// Marshal to XML with indentation
	data, err := xml.MarshalIndent(library, "", "  ")
	if err != nil {
		return err
	}

	// Add XML declaration
	xmlData := []byte(xml.Header + string(data))

	// Write to file with proper permissions (0644)
	if err := os.WriteFile(path, xmlData, 0644); err != nil {
		return err
	}

	return nil
}

// IsUnread returns true if the book has not been fully read.
// A book is unread if it has never been opened, or if LastPageRead
// hasn't reached the final page of the book.
func (b *ComicBook) IsUnread() bool {
	if b.OpenCount == 0 {
		return true
	}
	if b.PageCount > 0 {
		return b.LastPageRead < b.PageCount-1
	}
	return false
}

// GetBook returns a book by ID
func (l *ComicLibrary) GetBook(id string) *ComicBook {
	for i := range l.Books {
		if l.Books[i].ID == id {
			return &l.Books[i]
		}
	}
	return nil
}

// GetBooksByList returns books referenced in a reading list
func (l *ComicLibrary) GetBooksByList(list *ComicListItem) []*ComicBook {
	if list == nil {
		return nil
	}

	books := make([]*ComicBook, 0, len(list.Items))
	for _, item := range list.Items {
		if book := l.GetBook(item.ID); book != nil {
			books = append(books, book)
		}
	}
	return books
}

// FindList finds a reading list by name
func (l *ComicLibrary) FindList(name string) *ComicListItem {
	for i := range l.ComicLists {
		if l.ComicLists[i].Name == name {
			return &l.ComicLists[i]
		}
		// Recursively search in folders
		if found := findListRecursive(&l.ComicLists[i], name); found != nil {
			return found
		}
	}
	return nil
}

func findListRecursive(item *ComicListItem, name string) *ComicListItem {
	for i := range item.ChildItems {
		if item.ChildItems[i].Name == name {
			return &item.ChildItems[i]
		}
		if found := findListRecursive(&item.ChildItems[i], name); found != nil {
			return found
		}
	}
	return nil
}

// FindListByID finds a list by ID, searching recursively through folders
func (l *ComicLibrary) FindListByID(id string) *ComicListItem {
	for i := range l.ComicLists {
		if l.ComicLists[i].ID == id {
			return &l.ComicLists[i]
		}
		// Recursively search in folders
		if found := findListByIDRecursive(&l.ComicLists[i], id); found != nil {
			return found
		}
	}
	return nil
}

func findListByIDRecursive(item *ComicListItem, id string) *ComicListItem {
	for i := range item.ChildItems {
		if item.ChildItems[i].ID == id {
			return &item.ChildItems[i]
		}
		if found := findListByIDRecursive(&item.ChildItems[i], id); found != nil {
			return found
		}
	}
	return nil
}
