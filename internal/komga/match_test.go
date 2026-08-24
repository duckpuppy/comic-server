package komga

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/duckpuppy/comic-server/internal/library"
)

// TranslatePath's actual logic is tested in internal/pathmap; this package
// only re-exports a thin wrapper (see match.go), so a single delegation
// smoke test is enough here.
func TestTranslatePath_DelegatesToPathmap(t *testing.T) {
	got, err := TranslatePath(`G:\Comics\`, "/data", `G:\Comics\Batman\Batman #1.cbz`)
	if err != nil {
		t.Fatalf("TranslatePath() error = %v", err)
	}
	if want := "/data/Batman/Batman #1.cbz"; got != want {
		t.Errorf("TranslatePath() = %q, want %q", got, want)
	}
}

func TestResolveReadListBooks(t *testing.T) {
	idx := &Index{
		booksByPath: map[string]string{
			"/data/Batman/Batman #1.cbz": "book-1",
			"/data/Batman/Batman #2.cbz": "book-2",
		},
	}

	books := []*library.ComicBook{
		{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`},
		{ID: "2", FilePath: `G:\Comics\Batman\Batman #2.cbz`},
		{ID: "3", FilePath: `G:\Comics\Batman\Batman #3.cbz`}, // not in Komga index
		{ID: "4", FilePath: `D:\Elsewhere\Batman #4.cbz`},     // not rooted at local_root
	}

	matched, unmatched := idx.ResolveReadListBooks(books, `G:\Comics\`, "/data")

	if len(matched) != 2 {
		t.Fatalf("expected 2 matched books, got %d: %v", len(matched), matched)
	}
	if matched[0] != "book-1" || matched[1] != "book-2" {
		t.Errorf("unexpected matched order/content: %v", matched)
	}
	if len(unmatched) != 2 {
		t.Fatalf("expected 2 unmatched books, got %d", len(unmatched))
	}
	if unmatched[0].Book.ID != "3" || unmatched[1].Book.ID != "4" {
		t.Errorf("unexpected unmatched books: %+v", unmatched)
	}
	for _, u := range unmatched {
		if u.Reason == "" {
			t.Errorf("expected non-empty reason for unmatched book %s", u.Book.ID)
		}
	}
}

func TestResolveReadListBooks_DeduplicatesSameKomgaID(t *testing.T) {
	idx := &Index{
		booksByPath: map[string]string{
			"/data/Batman/Batman #1.cbz": "book-1",
		},
	}
	// Same underlying file referenced twice (e.g. duplicate library entries).
	books := []*library.ComicBook{
		{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`},
		{ID: "1-dup", FilePath: `G:\Comics\Batman\Batman #1.cbz`},
	}

	matched, unmatched := idx.ResolveReadListBooks(books, `G:\Comics\`, "/data")
	if len(matched) != 1 {
		t.Errorf("expected deduplication to 1 matched ID, got %d: %v", len(matched), matched)
	}
	if len(unmatched) != 0 {
		t.Errorf("expected no unmatched books, got %d", len(unmatched))
	}
}

func TestResolveBookReadStatus(t *testing.T) {
	idx := &Index{
		booksByPath: map[string]string{
			"/data/Batman/Batman #1.cbz": "book-1",
			"/data/Batman/Batman #2.cbz": "book-2",
		},
	}

	books := []*library.ComicBook{
		// Read: opened, and on/past the last page.
		{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`, OpenCount: 1, PageCount: 10, LastPageRead: 9},
		// Unread: never opened.
		{ID: "2", FilePath: `G:\Comics\Batman\Batman #2.cbz`, OpenCount: 0},
		// Not in Komga's index.
		{ID: "3", FilePath: `G:\Comics\Batman\Batman #3.cbz`},
	}

	matched, unmatched := idx.ResolveBookReadStatus(books, `G:\Comics\`, "/data")

	if len(matched) != 2 {
		t.Fatalf("expected 2 matched books, got %d: %+v", len(matched), matched)
	}
	if matched[0].KomgaBookID != "book-1" || !matched[0].Read {
		t.Errorf("expected book-1 matched as Read, got %+v", matched[0])
	}
	if matched[1].KomgaBookID != "book-2" || matched[1].Read {
		t.Errorf("expected book-2 matched as unread, got %+v", matched[1])
	}
	if matched[0].Book.ID != "1" || matched[1].Book.ID != "2" {
		t.Errorf("expected matched entries to carry their source book, got %+v", matched)
	}

	if len(unmatched) != 1 || unmatched[0].Book.ID != "3" {
		t.Errorf("expected book 3 unmatched, got %+v", unmatched)
	}
}

func TestResolveCollectionSeries(t *testing.T) {
	idx := &Index{
		seriesByPath: map[string]string{
			"/data/Batman":   "series-batman",
			"/data/Superman": "series-superman",
		},
	}

	books := []*library.ComicBook{
		{ID: "1", Series: "Batman", FilePath: `G:\Comics\Batman\Batman #1.cbz`},
		{ID: "2", Series: "Batman", FilePath: `G:\Comics\Batman\Batman #2.cbz`}, // same series as book 1
		{ID: "3", Series: "Superman", FilePath: `G:\Comics\Superman\Superman #1.cbz`},
		{ID: "4", Series: "Unknown", FilePath: `G:\Comics\Unknown\Unknown #1.cbz`}, // not in Komga index
	}

	matched, unmatched := idx.ResolveCollectionSeries(books, `G:\Comics\`, "/data")

	if len(matched) != 2 {
		t.Fatalf("expected 2 distinct matched series, got %d: %v", len(matched), matched)
	}
	if matched[0] != "series-batman" || matched[1] != "series-superman" {
		t.Errorf("unexpected matched series order/content: %v", matched)
	}
	if len(unmatched) != 1 || unmatched[0].Book.ID != "4" {
		t.Fatalf("unexpected unmatched: %+v", unmatched)
	}
}

func TestBuildIndex(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{
				Content: []Series{{ID: "s1", Name: "Batman", URL: "/data/Batman"}},
				Last:    true,
			})
		case "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{
				Content: []Book{{ID: "b1", SeriesID: "s1", URL: "/data/Batman/Batman #1.cbz"}},
				Last:    true,
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})

	idx, err := BuildIndex(context.Background(), c)
	if err != nil {
		t.Fatalf("BuildIndex() error = %v", err)
	}
	if idx.seriesByPath["/data/Batman"] != "s1" {
		t.Errorf("series index missing expected entry: %+v", idx.seriesByPath)
	}
	if idx.booksByPath["/data/Batman/Batman #1.cbz"] != "b1" {
		t.Errorf("book index missing expected entry: %+v", idx.booksByPath)
	}
}
