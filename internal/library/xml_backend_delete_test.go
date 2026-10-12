package library

import "testing"

func TestXMLBackend_DeleteBook(t *testing.T) {
	b := NewXMLBackendFromLibrary(&ComicLibrary{Books: []ComicBook{{ID: "1"}, {ID: "2"}, {ID: "3"}}}, "", nil)

	if err := b.DeleteBook("2"); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	all, _ := b.GetAllBooks()
	if len(all) != 2 || all[0].ID != "1" || all[1].ID != "3" {
		t.Errorf("books = %+v, want 1,3 in order", all)
	}
	if got, _ := b.GetBook("2"); got != nil {
		t.Error("deleted book still found")
	}
	if err := b.DeleteBook("2"); err == nil {
		t.Error("deleting a missing book should error")
	}
}
