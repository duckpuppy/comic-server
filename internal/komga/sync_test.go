package komga

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
)

// fakeBackend is a minimal library.Backend for testing Syncer without a
// real library XML file. Only FindListByID/MatchBooks are exercised by
// Syncer; everything else is unused stubs.
type fakeBackend struct {
	lists map[string]*library.ComicListItem
	books map[string][]*library.ComicBook // keyed by list ID
}

func (f *fakeBackend) GetBook(id string) (*library.ComicBook, error) { return nil, nil }
func (f *fakeBackend) GetAllBooks() ([]library.ComicBook, error)     { return nil, nil }
func (f *fakeBackend) FindListByID(id string) (*library.ComicListItem, error) {
	list, ok := f.lists[id]
	if !ok {
		return nil, fmt.Errorf("list %s not found", id)
	}
	return list, nil
}
func (f *fakeBackend) FindList(name string) (*library.ComicListItem, error) { return nil, nil }
func (f *fakeBackend) GetAllLists() ([]library.ComicListItem, error)        { return nil, nil }
func (f *fakeBackend) CreateList(list *library.ComicListItem) error         { return nil }
func (f *fakeBackend) UpdateList(list *library.ComicListItem) error         { return nil }
func (f *fakeBackend) DeleteList(id string) error                           { return nil }
func (f *fakeBackend) MoveList(id, parentID string) error                   { return nil }
func (f *fakeBackend) MatchBooks(list *library.ComicListItem) ([]*library.ComicBook, error) {
	return f.books[list.ID], nil
}
func (f *fakeBackend) GetBooksForList(list *library.ComicListItem) ([]*library.ComicBook, error) {
	return f.books[list.ID], nil
}
func (f *fakeBackend) UpdateBook(book *library.ComicBook) error     { return nil }
func (f *fakeBackend) UpdateBooks(books []*library.ComicBook) error { return nil }
func (f *fakeBackend) MarkDirty(bookID string)                      {}
func (f *fakeBackend) MarkManyDirty(bookIDs []string)               {}
func (f *fakeBackend) Flush() error                                 { return nil }
func (f *fakeBackend) Close() error                                 { return nil }
func (f *fakeBackend) LibraryID() string                            { return "test-library" }
func (f *fakeBackend) LibraryName() string                          { return "Test Library" }
func (f *fakeBackend) BookCount() int                               { return len(f.books) }
func (f *fakeBackend) CanPersist() bool                             { return false }

func TestSyncer_SyncTarget_Collection(t *testing.T) {
	var upsertedName string
	var upsertedSeriesIDs []string

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{
				Content: []Series{{ID: "s1", Name: "Batman", URL: "/data/Batman"}},
				Last:    true,
			})
		case r.URL.Path == "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{Last: true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/collections":
			json.NewEncoder(w).Encode(pageResponse[collectionDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/collections":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			upsertedName, _ = body["name"].(string)
			if ids, ok := body["seriesIds"].([]any); ok {
				for _, id := range ids {
					upsertedSeriesIDs = append(upsertedSeriesIDs, id.(string))
				}
			}
			json.NewEncoder(w).Encode(collectionDto{ID: "new-id"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-1}": {ID: "{GUID-1}", Name: "Batman Comics"},
		},
		books: map[string][]*library.ComicBook{
			"{GUID-1}": {{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`}},
		},
	}

	syncer := &Syncer{
		client:  c,
		backend: backend,
		opts: SyncOptions{
			LocalRoot:  `G:\Comics\`,
			RemoteRoot: "/data",
			Targets: []Target{
				{ListID: "{GUID-1}", KomgaName: "Batman Collection", Type: TargetCollection},
			},
		},
	}

	var results []TargetResult
	syncer.syncOnce(context.Background(), func(r TargetResult) { results = append(results, r) })

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].MatchedCount != 1 {
		t.Errorf("expected 1 matched series, got %d", results[0].MatchedCount)
	}
	if results[0].SourceBookCount != 1 {
		t.Errorf("expected SourceBookCount 1, got %d", results[0].SourceBookCount)
	}
	if upsertedName != "Batman Collection" {
		t.Errorf("unexpected upserted collection name: %q", upsertedName)
	}
	if len(upsertedSeriesIDs) != 1 || upsertedSeriesIDs[0] != "s1" {
		t.Errorf("unexpected upserted series IDs: %v", upsertedSeriesIDs)
	}
}

// TestSyncer_SyncTarget_Collection_SourceBookCountTracksDedup covers the
// user-reported case (2026-08-24): a Collection target with many issues per
// series legitimately produces MatchedCount << SourceBookCount (series
// dedup), which the UI needs SourceBookCount to explain clearly instead of
// looking like most of the list failed to match.
func TestSyncer_SyncTarget_Collection_SourceBookCountTracksDedup(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{
				Content: []Series{{ID: "s1", Name: "Batman", URL: "/data/Batman"}},
				Last:    true,
			})
		case r.URL.Path == "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{Last: true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/collections":
			json.NewEncoder(w).Encode(pageResponse[collectionDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/collections":
			json.NewEncoder(w).Encode(collectionDto{ID: "new-id"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-1}": {ID: "{GUID-1}", Name: "Batman Comics"},
		},
		books: map[string][]*library.ComicBook{
			"{GUID-1}": {
				{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`},
				{ID: "2", FilePath: `G:\Comics\Batman\Batman #2.cbz`},
				{ID: "3", FilePath: `G:\Comics\Batman\Batman #3.cbz`},
			},
		},
	}

	syncer := &Syncer{
		client:  c,
		backend: backend,
		opts: SyncOptions{
			LocalRoot:  `G:\Comics\`,
			RemoteRoot: "/data",
			Targets: []Target{
				{ListID: "{GUID-1}", KomgaName: "Batman Collection", Type: TargetCollection},
			},
		},
	}

	var results []TargetResult
	syncer.syncOnce(context.Background(), func(r TargetResult) { results = append(results, r) })

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].MatchedCount != 1 {
		t.Errorf("expected 1 matched series, got %d", results[0].MatchedCount)
	}
	if results[0].SourceBookCount != 3 {
		t.Errorf("expected SourceBookCount 3 (3 issues), got %d", results[0].SourceBookCount)
	}
	if len(results[0].Unmatched) != 0 {
		t.Errorf("expected 0 unmatched, got %d", len(results[0].Unmatched))
	}
}

func TestSyncer_SyncTarget_ReadList(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
		case r.URL.Path == "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{
				Content: []Book{{ID: "b1", URL: "/data/Batman/Batman #1.cbz"}},
				Last:    true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(pageResponse[readListDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(readListDto{ID: "new-id"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-2}": {ID: "{GUID-2}", Name: "Unread"},
		},
		books: map[string][]*library.ComicBook{
			"{GUID-2}": {
				{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`},
				{ID: "2", FilePath: `G:\Comics\NotInKomga\Missing.cbz`},
			},
		},
	}

	syncer := &Syncer{
		client:  c,
		backend: backend,
		opts: SyncOptions{
			LocalRoot:  `G:\Comics\`,
			RemoteRoot: "/data",
			Targets: []Target{
				{ListID: "{GUID-2}", KomgaName: "Unread", Type: TargetReadList},
			},
		},
	}

	var results []TargetResult
	syncer.syncOnce(context.Background(), func(r TargetResult) { results = append(results, r) })

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].MatchedCount != 1 {
		t.Errorf("expected 1 matched book, got %d", results[0].MatchedCount)
	}
	if len(results[0].Unmatched) != 1 || results[0].Unmatched[0].Book.ID != "2" {
		t.Errorf("expected book 2 to be unmatched, got %+v", results[0].Unmatched)
	}
}

// TestSyncer_SyncTarget_PushesReadStatus covers comic-server-bkh: when a
// target has SyncReadStatus enabled, each resolved book's read/unread state
// (per library.ComicBook.IsUnread) is pushed to Komga independently of the
// read-list/collection membership push - one PATCH per read book, one
// DELETE per unread book.
func TestSyncer_SyncTarget_PushesReadStatus(t *testing.T) {
	var patchedIDs, deletedIDs []string

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
		case r.URL.Path == "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{
				Content: []Book{
					{ID: "b1", URL: "/data/Batman/Batman #1.cbz"},
					{ID: "b2", URL: "/data/Batman/Batman #2.cbz"},
				},
				Last: true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(pageResponse[readListDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(readListDto{ID: "new-id"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/books/b1/read-progress":
			patchedIDs = append(patchedIDs, "b1")
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/books/b2/read-progress":
			deletedIDs = append(deletedIDs, "b2")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-3}": {ID: "{GUID-3}", Name: "Currently Reading"},
		},
		books: map[string][]*library.ComicBook{
			"{GUID-3}": {
				// Read: opened, on the last page.
				{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`, OpenCount: 1, PageCount: 10, LastPageRead: 9},
				// Unread: never opened.
				{ID: "2", FilePath: `G:\Comics\Batman\Batman #2.cbz`, OpenCount: 0},
			},
		},
	}

	syncer := &Syncer{
		client:  c,
		backend: backend,
		opts: SyncOptions{
			LocalRoot:  `G:\Comics\`,
			RemoteRoot: "/data",
			Targets: []Target{
				{ListID: "{GUID-3}", KomgaName: "Currently Reading", Type: TargetReadList, SyncReadStatus: true},
			},
		},
	}

	var results []TargetResult
	syncer.syncOnce(context.Background(), func(r TargetResult) { results = append(results, r) })

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].ReadStatusPushed != 2 {
		t.Errorf("expected 2 read-status pushes, got %d (failed: %+v)", results[0].ReadStatusPushed, results[0].ReadStatusFailed)
	}
	if len(results[0].ReadStatusFailed) != 0 {
		t.Errorf("expected no read-status failures, got %+v", results[0].ReadStatusFailed)
	}
	if len(patchedIDs) != 1 || patchedIDs[0] != "b1" {
		t.Errorf("expected exactly one PATCH for b1 (read), got %v", patchedIDs)
	}
	if len(deletedIDs) != 1 || deletedIDs[0] != "b2" {
		t.Errorf("expected exactly one DELETE for b2 (unread), got %v", deletedIDs)
	}
}

// TestSyncer_SyncTarget_ReadStatusDisabledByDefault confirms a target
// without SyncReadStatus set never touches the read-progress endpoints,
// even though every book resolves fine - the toggle must be opt-in.
func TestSyncer_SyncTarget_ReadStatusDisabledByDefault(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/series":
			json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
		case r.URL.Path == "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Book]{
				Content: []Book{{ID: "b1", URL: "/data/Batman/Batman #1.cbz"}},
				Last:    true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(pageResponse[readListDto]{Last: true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/readlists":
			json.NewEncoder(w).Encode(readListDto{ID: "new-id"})
		default:
			t.Errorf("unexpected request (read-progress should never be called): %s %s", r.Method, r.URL.Path)
		}
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-4}": {ID: "{GUID-4}", Name: "Currently Reading"},
		},
		books: map[string][]*library.ComicBook{
			"{GUID-4}": {{ID: "1", FilePath: `G:\Comics\Batman\Batman #1.cbz`, OpenCount: 1, PageCount: 10, LastPageRead: 9}},
		},
	}

	syncer := &Syncer{
		client:  c,
		backend: backend,
		opts: SyncOptions{
			LocalRoot:  `G:\Comics\`,
			RemoteRoot: "/data",
			Targets: []Target{
				{ListID: "{GUID-4}", KomgaName: "Currently Reading", Type: TargetReadList},
			},
		},
	}

	var results []TargetResult
	syncer.syncOnce(context.Background(), func(r TargetResult) { results = append(results, r) })

	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected result: %+v", results)
	}
	if results[0].ReadStatusPushed != 0 || len(results[0].ReadStatusFailed) != 0 {
		t.Errorf("expected no read-status activity when SyncReadStatus is unset, got pushed=%d failed=%+v", results[0].ReadStatusPushed, results[0].ReadStatusFailed)
	}
}

func TestSyncer_UnknownListID(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/series", "/api/v1/books":
			json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
		}
	})

	backend := &fakeBackend{lists: map[string]*library.ComicListItem{}}
	syncer := &Syncer{
		client:  c,
		backend: backend,
		opts: SyncOptions{
			Targets: []Target{{ListID: "{does-not-exist}", KomgaName: "X", Type: TargetCollection}},
		},
	}

	var results []TargetResult
	syncer.syncOnce(context.Background(), func(r TargetResult) { results = append(results, r) })

	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("expected an error result for unknown list ID, got %+v", results)
	}
}

func TestSyncer_Run_StopsOnContextCancel(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
	})

	backend := &fakeBackend{lists: map[string]*library.ComicListItem{}}
	syncer := NewSyncer(backend, SyncOptions{Interval: 10 * time.Millisecond})
	syncer.client = c // use the mock instead of a real client

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var runs int
	done := make(chan struct{})
	go func() {
		syncer.Run(ctx, func(r TargetResult) { runs++ })
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestSyncer_TriggerNow_CausesImmediateExtraSync(t *testing.T) {
	var syncCount int32

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/series" {
			atomic.AddInt32(&syncCount, 1)
		}
		json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-1}": {ID: "{GUID-1}", Name: "Batman Comics"},
		},
		books: map[string][]*library.ComicBook{},
	}
	// Long interval - only TriggerNow should cause the second sync within the test window.
	// A target must be configured, since syncOnce now skips BuildIndex
	// entirely when there are no targets to push (avoids needless Komga API
	// calls while idle - see comic-server-d3w).
	syncer := NewSyncer(backend, SyncOptions{
		Interval: time.Hour,
		Targets:  []Target{{ListID: "{GUID-1}", KomgaName: "Batman Collection", Type: TargetCollection}},
	})
	syncer.client = c

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		syncer.Run(ctx, nil)
		close(done)
	}()

	// Wait for the initial immediate sync from Run() itself.
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&syncCount) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&syncCount) < 1 {
		t.Fatal("expected at least 1 sync from Run()'s initial pass")
	}

	syncer.TriggerNow()

	deadline = time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&syncCount) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&syncCount); got < 2 {
		t.Errorf("expected TriggerNow to cause a 2nd sync pass, got %d total passes", got)
	}

	<-done
}

func TestSyncer_TriggerNow_CoalescesBeforeRunStarts(t *testing.T) {
	backend := &fakeBackend{lists: map[string]*library.ComicListItem{}}
	syncer := NewSyncer(backend, SyncOptions{})

	// Calling TriggerNow before Run starts (buffered channel of size 1)
	// must not block or panic, and a second call should coalesce rather
	// than block.
	syncer.TriggerNow()
	syncer.TriggerNow()
}

// TestSyncer_SetTargets verifies SetTargets replaces the live target set
// used by syncOnce, without needing a new Syncer or a config reload - the
// mechanism the web UI's Komga target endpoints rely on (comic-server-d3w).
func TestSyncer_SetTargets(t *testing.T) {
	var seriesRequests int32
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/series" {
			atomic.AddInt32(&seriesRequests, 1)
		}
		json.NewEncoder(w).Encode(pageResponse[Series]{Last: true})
	})

	backend := &fakeBackend{
		lists: map[string]*library.ComicListItem{
			"{GUID-1}": {ID: "{GUID-1}", Name: "Batman Comics"},
		},
		books: map[string][]*library.ComicBook{},
	}

	syncer := NewSyncer(backend, SyncOptions{})
	syncer.client = c

	// No targets configured yet - syncOnce should skip BuildIndex entirely.
	syncer.syncOnce(context.Background(), nil)
	if got := atomic.LoadInt32(&seriesRequests); got != 0 {
		t.Fatalf("expected 0 requests with no targets, got %d", got)
	}

	if got := syncer.Targets(); len(got) != 0 {
		t.Fatalf("expected Targets() to start empty, got %+v", got)
	}

	syncer.SetTargets([]Target{
		{ListID: "{GUID-1}", KomgaName: "Batman Collection", Type: TargetCollection},
	})

	if got := syncer.Targets(); len(got) != 1 || got[0].ListID != "{GUID-1}" {
		t.Fatalf("expected Targets() to reflect SetTargets, got %+v", got)
	}

	syncer.syncOnce(context.Background(), nil)
	if got := atomic.LoadInt32(&seriesRequests); got != 1 {
		t.Fatalf("expected SetTargets to take effect on the next syncOnce, got %d requests", got)
	}
}

func TestNewSyncer_DefaultsInterval(t *testing.T) {
	backend := &fakeBackend{}
	syncer := NewSyncer(backend, SyncOptions{})
	if syncer.opts.Interval != defaultSyncInterval {
		t.Errorf("expected default interval %v, got %v", defaultSyncInterval, syncer.opts.Interval)
	}
}
