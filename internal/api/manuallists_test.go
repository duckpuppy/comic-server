package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/audit"
	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/storage"
)

func TestInsertMembers(t *testing.T) {
	cases := []struct {
		cur, add  string
		index     int
		want, new string
	}{
		{"a,b", "c", 2, "a,b,c", "c"},
		{"a,b", "c,d", 0, "c,d,a,b", "c,d"},
		{"a,b", "b,c", 1, "a,c,b", "c"}, // already a member: skipped, not moved
		{"a,b", "c,c", 99, "a,b,c", "c"},
		{"a,b", "c", -5, "c,a,b", "c"},
		{"", "a", 0, "a", "a"},
		{"a", "a", 0, "a", ""},
	}
	for _, c := range cases {
		got, fresh := insertMembers(split(c.cur), split(c.add), c.index)
		if strings.Join(got, ",") != c.want || strings.Join(fresh, ",") != c.new {
			t.Errorf("insert(%q, %q, %d) = %v %v, want %s / %s", c.cur, c.add, c.index, got, fresh, c.want, c.new)
		}
	}
}

func TestMoveMember(t *testing.T) {
	cases := []struct {
		cur, id string
		index   int
		want    string
		at      int
		ok      bool
	}{
		{"a,b,c,d", "a", 2, "b,c,a,d", 2, true},
		{"a,b,c,d", "d", 0, "d,a,b,c", 0, true},
		{"a,b,c,d", "b", 99, "a,c,d,b", 3, true},
		{"a,b,c,d", "c", -1, "c,a,b,d", 0, true},
		{"a,b,c,d", "x", 0, "a,b,c,d", 0, false},
	}
	for _, c := range cases {
		cur := split(c.cur)
		got, at, ok := moveMember(cur, c.id, c.index)
		if strings.Join(got, ",") != c.want || at != c.at || ok != c.ok {
			t.Errorf("move(%q, %q, %d) = %v %d %v, want %s %d %v", c.cur, c.id, c.index, got, at, ok, c.want, c.at, c.ok)
		}
		if strings.Join(cur, ",") != c.cur {
			t.Errorf("move changed its input: %v", cur)
		}
	}
}

func TestRemoveMembers(t *testing.T) {
	got, removed := removeMembers(split("a,b,c"), split("c,x,a"))
	if strings.Join(got, ",") != "b" || strings.Join(removed, ",") != "a,c" {
		t.Errorf("remove = %v %v", got, removed)
	}
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func manualListBooks() []library.ComicBook {
	return []library.ComicBook{
		{ID: "b1", Series: "Batman", Number: "1", FilePath: "/c/1.cbz"},
		{ID: "b2", Series: "Batman", Number: "2", FilePath: "/c/2.cbz"},
		{ID: "b3", Series: "Superman", Number: "1", FilePath: "/c/3.cbz"},
		{ID: "b4", Series: "Superman", Number: "2", FilePath: "/c/4.cbz"},
	}
}

// manualListLists: on XML, To Read also holds a member that is no longer a
// book and a repeated member (SQLite's foreign key and primary key on
// reading_list_items allow neither).
func manualListLists(xml bool) []library.ComicListItem {
	members := []string{"b3", "b1"}
	if xml {
		members = []string{"b3", "gone", "b1", "b3"}
	}
	return []library.ComicListItem{
		{ID: "{smart}", Name: "Batman", Type: "ComicSmartListItem", MatcherMode: "And",
			Matchers: []library.ComicBookMatcher{{Type: "Series", MatchOperator: "0", MatchValue: "Batman"}}},
		{ID: "{manual}", Name: "To Read", Type: "ComicIdListItem", BookIds: members},
		{ID: "{folder}", Name: "Folder", Type: "ComicListItemFolder", ChildItems: []library.ComicListItem{
			{ID: "{nested}", Name: "another", Type: "ComicIdListItem", BookIds: []string{"b2"}},
		}},
	}
}

// manualListServers returns a test server per backend, each with the same
// books and lists and a config.db for the audit log.
func manualListServers(t *testing.T) map[string]*Server {
	t.Helper()
	newConfigDB := func() *configdb.DB {
		db, err := configdb.Open(filepath.Join(t.TempDir(), "config.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}

	xmlPath := filepath.Join(t.TempDir(), "ComicDb.xml")
	xb := library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: manualListBooks(), ComicLists: manualListLists(true)}, xmlPath, nil)

	sb, err := storage.NewSQLiteBackend(filepath.Join(t.TempDir(), "library.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sb.Close() })
	if _, err := sb.DB().Import(&library.ComicLibrary{ID: "lib", Books: manualListBooks(), ComicLists: manualListLists(false)}, storage.ImportOptions{}); err != nil {
		t.Fatal(err)
	}

	return map[string]*Server{
		"xml":    {backend: xb, listCache: library.NewListCache(0), configDB: newConfigDB()},
		"sqlite": {backend: sb, listCache: library.NewListCache(0), configDB: newConfigDB()},
	}
}

func postManualList(t *testing.T, s *Server, path, body string) (int, ManualListResult, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleListsRouter(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	var res ManualListResult
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, res, w.Body.String()
}

func listMemberIDs(t *testing.T, s *Server, listID string) string {
	t.Helper()
	l, err := s.backend.FindListByID(listID)
	if err != nil || l == nil {
		t.Fatalf("FindListByID(%s): %v %v", listID, l, err)
	}
	return strings.Join(l.BookIds, ",")
}

func TestManualList_AddRemoveMove(t *testing.T) {
	for name, s := range manualListServers(t) {
		t.Run(name, func(t *testing.T) {
			// Add: b3 is already in, x is not a book. On XML the save also
			// prunes the missing "gone" member and the repeated b3.
			code, res, body := postManualList(t, s, "/api/library/lists/{manual}/books", `{"book_ids":["b2","b3","x","b4"]}`)
			if code != http.StatusOK {
				t.Fatalf("add: %d %s", code, body)
			}
			if res.Added != 2 || res.Already != 1 || res.Unknown != 1 || res.BookCount != 4 {
				t.Errorf("add result = %+v", res)
			}
			if got := listMemberIDs(t, s, "{manual}"); got != "b3,b1,b2,b4" {
				t.Errorf("after add = %s", got)
			}

			// Move b4 to the top, then b3 to the end.
			if code, res, body := postManualList(t, s, "/api/library/lists/{manual}/books/move", `{"book_id":"b4","index":0}`); code != http.StatusOK || res.Position == nil || *res.Position != 0 {
				t.Fatalf("move: %d %+v %s", code, res, body)
			}
			if code, res, body := postManualList(t, s, "/api/library/lists/{manual}/books/move", `{"book_id":"b3","index":99}`); code != http.StatusOK || *res.Position != 3 {
				t.Fatalf("move to end: %d %+v %s", code, res, body)
			}
			if got := listMemberIDs(t, s, "{manual}"); got != "b4,b1,b2,b3" {
				t.Errorf("after move = %s", got)
			}

			// The preview shows the same order.
			w := httptest.NewRecorder()
			s.handleListsRouter(w, httptest.NewRequest(http.MethodGet, "/api/library/lists/{manual}/preview", nil))
			var pv struct{ Comics []ComicPreview }
			_ = json.Unmarshal(w.Body.Bytes(), &pv)
			var order []string
			for _, c := range pv.Comics {
				order = append(order, c.ID)
			}
			if strings.Join(order, ",") != "b4,b1,b2,b3" {
				t.Errorf("preview order = %v", order)
			}

			// Insert at a position.
			if code, _, body := postManualList(t, s, "/api/library/lists/{nested}/books", `{"book_ids":["b1"],"index":0}`); code != http.StatusOK {
				t.Fatalf("add to nested: %d %s", code, body)
			}
			if got := listMemberIDs(t, s, "{nested}"); got != "b1,b2" {
				t.Errorf("nested after insert = %s", got)
			}

			code, res, body = postManualList(t, s, "/api/library/lists/{manual}/books/remove", `{"book_ids":["b1","x"]}`)
			if code != http.StatusOK || res.Removed != 1 || res.BookCount != 3 {
				t.Fatalf("remove: %d %+v %s", code, res, body)
			}
			if got := listMemberIDs(t, s, "{manual}"); got != "b4,b2,b3" {
				t.Errorf("after remove = %s", got)
			}
			// The cached count is the new one at once, not a stale value
			// waiting on a background refresh.
			if c, u, ok := s.listCache.GetStaleCounts("{manual}"); !ok || c != 3 || u != 3 {
				t.Errorf("cached counts = %d/%d (%v), want 3/3", c, u, ok)
			}

			if code, _, _ := postManualList(t, s, "/api/library/lists/{manual}/books/move", `{"book_id":"b1","index":0}`); code != http.StatusNotFound {
				t.Errorf("move of a non-member = %d, want 404", code)
			}

			// History: b2 and b4 added, b1 added to the nested list and
			// removed from To Read.
			rows, _, err := s.configDB.ListAudit(configdb.AuditFilter{Action: audit.ActionReadingList, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range rows {
				got = append(got, r.BookID+":"+r.Changes)
			}
			slices.Sort(got)
			want := []string{
				`b1:[{"field":"list","old":"","new":"another"}]`,
				`b1:[{"field":"list","old":"To Read","new":""}]`,
				`b2:[{"field":"list","old":"","new":"To Read"}]`,
				`b4:[{"field":"list","old":"","new":"To Read"}]`,
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("audit rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestManualList_AddFromBrowseScope(t *testing.T) {
	for name, s := range manualListServers(t) {
		t.Run(name, func(t *testing.T) {
			body := `{"browse":{"matcher_mode":"And","matchers":[{"Type":"Series","MatchOperator":"0","MatchValue":"Superman"}]}}`
			code, res, text := postManualList(t, s, "/api/library/lists/{nested}/books", body)
			if code != http.StatusOK || res.Added != 2 {
				t.Fatalf("add browse scope: %d %+v %s", code, res, text)
			}
			if got := listMemberIDs(t, s, "{nested}"); got != "b2,b3,b4" {
				t.Errorf("members = %s", got)
			}
		})
	}
}

func TestManualList_Refusals(t *testing.T) {
	s := manualListServers(t)["xml"]
	cases := []struct {
		path, body string
		want       int
	}{
		{"/api/library/lists/{smart}/books", `{"book_ids":["b1"]}`, http.StatusConflict},
		{"/api/library/lists/{missing}/books", `{"book_ids":["b1"]}`, http.StatusNotFound},
		{"/api/library/lists/{manual}/books", `{}`, http.StatusBadRequest},
		{"/api/library/lists/{manual}/books/remove", `{}`, http.StatusBadRequest},
		{"/api/library/lists/{manual}/books/move", `{"index":1}`, http.StatusBadRequest},
		{"/api/library/lists/{manual}/books/nope", `{}`, http.StatusNotFound},
	}
	for _, c := range cases {
		if code, _, body := postManualList(t, s, c.path, c.body); code != c.want {
			t.Errorf("POST %s %s = %d (%s), want %d", c.path, c.body, code, body, c.want)
		}
	}
	w := httptest.NewRecorder()
	s.handleListsRouter(w, httptest.NewRequest(http.MethodGet, "/api/library/lists/{manual}/books", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET books = %d", w.Code)
	}

	readOnly := &Server{backend: library.NewXMLBackendFromLibrary(&library.ComicLibrary{Books: manualListBooks(), ComicLists: manualListLists(true)}, "", nil), listCache: library.NewListCache(0)}
	if code, _, _ := postManualList(t, readOnly, "/api/library/lists/{manual}/books", `{"book_ids":["b1"]}`); code != http.StatusForbidden {
		t.Errorf("read-only library = %d, want 403", code)
	}
}

func TestHandleManualLists(t *testing.T) {
	for name, s := range manualListServers(t) {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.handleManualLists(w, httptest.NewRequest(http.MethodGet, "/api/library/manual-lists?book_id=b2", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			var resp struct{ Lists []ManualListSummary }
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			// Smart lists and folders are left out; nested lists are in;
			// sorted by name ignoring case.
			if len(resp.Lists) != 2 {
				t.Fatalf("lists = %+v", resp.Lists)
			}
			if resp.Lists[0].ID != "{nested}" || !resp.Lists[0].Contains || resp.Lists[0].BookCount != 1 {
				t.Errorf("first = %+v", resp.Lists[0])
			}
			if resp.Lists[1].ID != "{manual}" || resp.Lists[1].Contains || resp.Lists[1].BookCount != 2 {
				t.Errorf("second = %+v", resp.Lists[1])
			}
		})
	}
}
