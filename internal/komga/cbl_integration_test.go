package komga

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/duckpuppy/comic-server/internal/cbl"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

// TestSyncer_SyncTarget_CBLImportedReadingList is an end-to-end check for
// comic-server-hmld: does a CBL-imported reading list actually sync to
// Komga, and does a reimport's membership change actually propagate on
// the next sync? Every other Komga sync test in this package uses
// fakeBackend and a hand-built ComicListItem; this one goes through the
// real storage.SQLiteBackend and a real cbl.Parse -> storage.ImportCBL
// pipeline, since that's the part comic-server-zw0o added and nothing
// had exercised against Komga sync before.
func TestSyncer_SyncTarget_CBLImportedReadingList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sb, err := storage.NewSQLiteBackend(dbPath, "")
	if err != nil {
		t.Fatalf("create backend: %v", err)
	}
	defer sb.Close()

	batman := library.ComicBook{ID: "book-batman", FilePath: `G:\Comics\Batman\Batman #1.cbz`, Series: "Batman", Number: "1"}
	detective := library.ComicBook{ID: "book-detective", FilePath: `G:\Comics\Detective\Detective Comics #27.cbz`, Series: "Detective Comics", Number: "27"}
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: []library.ComicBook{batman, detective}}, storage.ImportOptions{}); err != nil {
		t.Fatalf("seed library: %v", err)
	}

	// Step 1: CBL import matching only Batman - proves GetBooksForList
	// resolves a freshly-imported CBL reading list's real membership,
	// not an empty/placeholder list.
	rl := &cbl.ReadingList{Name: "Unread", Books: []cbl.Book{
		{Series: "Batman", Number: "1"},
	}}
	importResult, err := sb.DB().ImportCBL(rl, storage.CBLImportSource{Source: "local_file"})
	if err != nil {
		t.Fatalf("ImportCBL: %v", err)
	}
	listID := importResult.ListID

	var lastPostedBookIDs []string
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
		case r.URL.Path == "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{
				Content: []Book{
					{ID: "komga-batman", URL: `/data/Batman/Batman #1.cbz`},
					{ID: "komga-detective", URL: `/data/Detective/Detective Comics #27.cbz`},
				},
				Last: true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(pageResponse[readListDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/readlists":
			var body struct {
				BookIds []string `json:"bookIds"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			lastPostedBookIDs = body.BookIds
			json.NewEncoder(w).Encode(readListDto{ID: "komga-readlist-1"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	syncer := &Syncer{
		client:  c,
		backend: sb,
		opts: SyncOptions{
			LocalRoot:  `G:\Comics\`,
			RemoteRoot: "/data",
			Targets:    []Target{{ListID: listID, KomgaName: "Unread", Type: TargetReadList}},
		},
	}

	var results []TargetResult
	collect := func(r TargetResult) {
		if r.Target.ListID == "" && r.Err == nil {
			return
		}
		results = append(results, r)
	}

	syncer.syncOnce(context.Background(), collect)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("first sync: results=%+v", results)
	}
	if results[0].MatchedCount != 1 {
		t.Fatalf("first sync: MatchedCount = %d, want 1 (Batman only)", results[0].MatchedCount)
	}
	if len(lastPostedBookIDs) != 1 || lastPostedBookIDs[0] != "komga-batman" {
		t.Fatalf("first sync: posted bookIds = %v, want [komga-batman]", lastPostedBookIDs)
	}

	// Step 2: reimport with an upstream CBL that now also includes
	// Detective Comics - proves the watch/reimport engine's full-replace
	// (comic-server-zw0o) actually changes what the NEXT Komga sync
	// sends, not just what's in the database.
	updatedRL := &cbl.ReadingList{Name: "Unread", Books: []cbl.Book{
		{Series: "Batman", Number: "1"},
		{Series: "Detective Comics", Number: "27"},
	}}
	if _, err := sb.DB().ReimportCBL(listID, updatedRL, storage.CBLImportSource{Source: "local_file"}); err != nil {
		t.Fatalf("ReimportCBL: %v", err)
	}

	results = nil
	syncer.syncOnce(context.Background(), collect)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("second sync: results=%+v", results)
	}
	if results[0].MatchedCount != 2 {
		t.Fatalf("second sync: MatchedCount = %d, want 2 (Batman + Detective Comics after reimport)", results[0].MatchedCount)
	}
	if len(lastPostedBookIDs) != 2 {
		t.Fatalf("second sync: posted bookIds = %v, want 2 entries", lastPostedBookIDs)
	}
}
