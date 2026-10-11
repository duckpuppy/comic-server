package cbzconvert

import (
	"encoding/xml"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/trash"
)

func richTestBook() *library.ComicBook {
	return &library.ComicBook{
		ID:                   "{real-library-id}",
		FilePath:             `C:\Comics\book.cbr`,
		FileSize:             12345,
		FileModifiedTime:     library.ComicTime{Time: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)},
		FileCreationTime:     library.ComicTime{Time: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)},
		LastOpenedFromListID: "list-1",
		ComicInfoIsDirty:     true,
		FileIsMissing:        true,
		NewPages:             3,

		Series:  "X-Men",
		Number:  "12",
		Title:   "Fatal Attractions",
		Year:    1992,
		Rating:  4.5,
		Checked: true,

		CustomValuesStore: ",comicvine_volume=123,DataManager.processed=1",
		BookPrice:         9.99,
		BookStore:         "Local Shop",
		ISBN:              "1234567890",

		PageCount:           6,
		CurrentPage:         4,
		LastPageRead:        4,
		PreferredFrontCover: 2,
		OpenCount:           2,
		OpenedTime:          library.ComicTime{Time: time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)},
		Pages: []library.ComicPageInfo{
			{Image: 0, Type: library.PageTypeStory},
			{Image: 1, Type: library.PageTypeDeleted},
			{Image: 2, Type: library.PageTypeFrontCover, Bookmark: "Cover"},
			{Image: 3, Type: library.PageTypeDeleted},
			{Image: 4, Type: library.PageTypeStory, Bookmark: "Fight"},
			{Image: 5, Type: library.PageTypeBackCover},
		},
	}
}

func TestBuildComicBookXML_RootAndExcludedFields(t *testing.T) {
	data, err := BuildComicBookXML(richTestBook(), 4)
	if err != nil {
		t.Fatalf("BuildComicBookXML: %v", err)
	}
	text := string(data)

	if !strings.HasPrefix(text, "<?xml") {
		t.Error("missing XML declaration")
	}
	if !strings.Contains(text, "<ComicBook ") && !strings.Contains(text, "<ComicBook>") {
		t.Errorf("root element should be ComicBook:\n%s", text)
	}

	for _, banned := range []string{"File=", "C:\\", "FileSize", "FileModifiedTime", "FileCreationTime", "LastOpenedFromListId", "ComicInfoIsDirty", "<Missing>", "NewPages", "real-library-id", "list-1"} {
		if strings.Contains(text, banned) {
			t.Errorf("ComicBook.xml must not embed %q:\n%s", banned, text)
		}
	}
	if !strings.Contains(text, `Id="`+zeroGUID+`"`) {
		t.Errorf("Id should be the zero GUID:\n%s", text)
	}
}

func TestBuildComicBookXML_RoundTripsLibraryOnlyData(t *testing.T) {
	data, err := BuildComicBookXML(richTestBook(), 4)
	if err != nil {
		t.Fatal(err)
	}
	var got library.ComicBook
	if err := xml.Unmarshal(data, &got); err != nil {
		t.Fatalf("not well-formed / not readable as a ComicBook: %v", err)
	}

	if got.Series != "X-Men" || got.Number != "12" || got.Year != 1992 || got.Rating != 4.5 || !got.Checked {
		t.Errorf("core metadata lost: %+v", got)
	}
	if got.CustomValuesStore != ",comicvine_volume=123,DataManager.processed=1" {
		t.Errorf("CustomValuesStore = %q", got.CustomValuesStore)
	}
	if got.BookPrice != 9.99 || got.BookStore != "Local Shop" || got.ISBN != "1234567890" {
		t.Errorf("catalog fields lost: price=%v store=%q isbn=%q", got.BookPrice, got.BookStore, got.ISBN)
	}
	if got.OpenCount != 2 || got.OpenedTime.IsZero() {
		t.Errorf("read progress lost: count=%d opened=%v", got.OpenCount, got.OpenedTime)
	}
	if got.ID != zeroGUID || got.FilePath != "" || got.FileSize != 0 {
		t.Errorf("file/library fields leaked: id=%q path=%q size=%d", got.ID, got.FilePath, got.FileSize)
	}
	if !got.EnableDynamicUpdate {
		t.Error("EnableDynamicUpdate should be true, as ComicBook.Serialize sets it")
	}
}

func TestBuildComicBookXML_RebasesPagesAfterDeletedPages(t *testing.T) {
	book := richTestBook()
	data, err := BuildComicBookXML(book, 4)
	if err != nil {
		t.Fatal(err)
	}
	var got library.ComicBook
	if err := xml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	// Source pages 0..5 with 1 and 3 deleted -> 4 pages: old 0,2,4,5 become 0,1,2,3.
	want := []library.ComicPageInfo{
		{Image: 0, Type: library.PageTypeStory},
		{Image: 1, Type: library.PageTypeFrontCover, Bookmark: "Cover"},
		{Image: 2, Type: library.PageTypeStory, Bookmark: "Fight"},
		{Image: 3, Type: library.PageTypeBackCover},
	}
	if len(got.Pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %+v", len(got.Pages), len(want), got.Pages)
	}
	for i, w := range want {
		g := got.Pages[i]
		if g.Image != w.Image || g.Type != w.Type || g.Bookmark != w.Bookmark {
			t.Errorf("page %d = %+v, want %+v", i, g, w)
		}
	}

	if got.PageCount != 4 {
		t.Errorf("PageCount = %d, want 4", got.PageCount)
	}
	// Old page 4 had two deleted pages before it -> new index 2.
	if got.CurrentPage != 2 || got.LastPageRead != 2 {
		t.Errorf("progress = current %d / lastRead %d, want 2/2", got.CurrentPage, got.LastPageRead)
	}
	// Old page 2 had one deleted page before it -> new index 1.
	if got.PreferredFrontCover != 1 {
		t.Errorf("PreferredFrontCover = %d, want 1", got.PreferredFrontCover)
	}

	// The input record must not be modified.
	if len(book.Pages) != 6 || book.ID != "{real-library-id}" || book.FilePath == "" {
		t.Error("BuildComicBookXML mutated its input")
	}
}

func TestBuildComicBookXML_ClampsProgressIntoRange(t *testing.T) {
	book := &library.ComicBook{CurrentPage: 99, LastPageRead: 99, PreferredFrontCover: 99}
	data, err := BuildComicBookXML(book, 3)
	if err != nil {
		t.Fatal(err)
	}
	var got library.ComicBook
	if err := xml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.CurrentPage != 2 || got.LastPageRead != 2 || got.PreferredFrontCover != 2 {
		t.Errorf("progress not clamped: %d/%d/%d", got.CurrentPage, got.LastPageRead, got.PreferredFrontCover)
	}
}

func TestConvert_EmbedComicBookOption(t *testing.T) {
	for _, embed := range []bool{false, true} {
		name := "off"
		if embed {
			name = "on"
		}
		t.Run(name, func(t *testing.T) {
			libDir := t.TempDir()
			src := filepath.Join(libDir, "book.cbz")
			writeTestCBZ(t, src, map[string][]byte{
				"page001.jpg": []byte("one"),
				"page002.jpg": []byte("two"),
				"page003.jpg": []byte("three"),
			})

			book := &library.ComicBook{
				FilePath:          src,
				Series:            "Test Series",
				CustomValuesStore: ",k=v",
				CurrentPage:       2,
				Pages: []library.ComicPageInfo{
					{Image: 0, Type: library.PageTypeStory},
					{Image: 1, Type: library.PageTypeDeleted},
					{Image: 2, Type: library.PageTypeStory},
				},
			}
			tr := &trash.Trash{Root: t.TempDir(), RetentionDays: 30}
			if _, err := Convert(book, nil, tr, Options{EmbedComicBook: embed}); err != nil {
				t.Fatalf("Convert: %v", err)
			}

			entries := readZipEntries(t, src)
			if _, ok := entries[comicInfoEntryName]; !ok {
				t.Fatal("ComicInfo.xml must always be written")
			}
			cbData, hasComicBook := entries[comicBookEntryName]
			if hasComicBook != embed {
				t.Fatalf("ComicBook.xml present = %v, want %v", hasComicBook, embed)
			}
			if !embed {
				return
			}
			var got library.ComicBook
			if err := xml.Unmarshal(cbData, &got); err != nil {
				t.Fatalf("embedded ComicBook.xml unreadable: %v", err)
			}
			if got.Series != "Test Series" || got.CustomValuesStore != ",k=v" {
				t.Errorf("embedded record wrong: %+v", got)
			}
			// 3 source pages, page 1 deleted -> 2 pages; current page 2 -> 1.
			if got.PageCount != 2 || got.CurrentPage != 1 {
				t.Errorf("PageCount=%d CurrentPage=%d, want 2 and 1", got.PageCount, got.CurrentPage)
			}
		})
	}
}
