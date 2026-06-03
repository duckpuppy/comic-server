package library

import (
	"encoding/xml"
	"testing"
)

func TestIdListItemParsing(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<ComicDatabase Id="test-lib" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Books>
    <Book Id="book-001" File="book1.cbz" />
    <Book Id="book-002" File="book2.cbz" />
    <Book Id="book-003" File="book3.cbz" />
  </Books>
  <ComicLists>
    <Item xsi:type="ComicIdListItem" Id="list-001" Name="My ID List">
      <BookCount>2</BookCount>
      <BookIds>
        <guid>book-001</guid>
        <guid>book-003</guid>
      </BookIds>
    </Item>
  </ComicLists>
</ComicDatabase>`

	var lib ComicLibrary
	if err := xml.Unmarshal([]byte(xmlData), &lib); err != nil {
		t.Fatalf("failed to parse XML: %v", err)
	}

	if len(lib.ComicLists) != 1 {
		t.Fatalf("expected 1 list, got %d", len(lib.ComicLists))
	}

	list := &lib.ComicLists[0]
	if list.Type != "ComicIdListItem" {
		t.Errorf("expected type ComicIdListItem, got %q", list.Type)
	}
	if len(list.BookIds) != 2 {
		t.Errorf("expected 2 BookIds, got %d: %v", len(list.BookIds), list.BookIds)
	}

	books, err := lib.GetBooksForList(list)
	if err != nil {
		t.Fatalf("GetBooksForList returned error: %v", err)
	}
	if len(books) != 2 {
		t.Errorf("expected 2 books, got %d", len(books))
	}

	ids := map[string]bool{}
	for _, b := range books {
		ids[b.ID] = true
	}
	if !ids["book-001"] || !ids["book-003"] {
		t.Errorf("unexpected book IDs: %v", ids)
	}
}
