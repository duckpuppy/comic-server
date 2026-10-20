package api

import (
	"net/http"
	"testing"

	"github.com/duckpuppy/comic-server/internal/configdb"
)

func TestReplaceText(t *testing.T) {
	cases := []struct {
		text, find, repl      string
		ignoreCase, wholeWord bool
		want                  string
	}{
		{"DC Comics", "DC", "Marvel", false, false, "Marvel Comics"},
		{"dc comics", "DC", "X", false, false, "dc comics"}, // case-sensitive by default
		{"dc comics", "DC", "X", true, false, "X comics"},
		{"Batman and Batmanga", "Batman", "Bat", false, true, "Bat and Batmanga"},
		{"Bat-Man", "Bat", "X", false, true, "X-Man"},
		{"a_bat", "bat", "X", false, true, "a_bat"}, // underscore is a word character
		{"(DC)", "DC", "X", false, true, "(X)"},
		{"a.b a.b", "a.b", "-", false, false, "- -"}, // no regex
		{"axb", "a.b", "-", false, false, "axb"},
		{"cost", "cost", "$1 \\1", false, false, "$1 \\1"}, // replacement is literal
		{"Éclair", "é", "E", true, false, "Eclair"},
		{"Éclair", "é", "E", true, true, "Éclair"}, // inside a word
		{"abc", "b", "", false, false, "ac"},
	}
	for _, c := range cases {
		if got := replaceText(c.text, c.find, c.repl, c.ignoreCase, c.wholeWord); got != c.want {
			t.Errorf("replaceText(%q,%q,%q,ic=%v,ww=%v) = %q, want %q", c.text, c.find, c.repl, c.ignoreCase, c.wholeWord, got, c.want)
		}
	}
}

func TestBulkEdit_FindReplacePreviewAndApply(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	edit := `"edit":{"find_replace":{"field":"Publisher","find":"dc","replace":"DC Comics","ignore_case":true,"whole_word":true}}`
	code, resp, raw := postBulkEdit(t, s, `{"book_ids":["b1","b2","b3"],"expected_count":3,`+edit+`}`)
	if code != http.StatusOK {
		t.Fatalf("preview: %d %s", code, raw)
	}
	if resp.Applied || resp.Changed != 3 || resp.Fields["Publisher"] != 3 {
		t.Errorf("preview resp = %+v", resp)
	}
	var diff *BulkEditDiff
	for _, sm := range resp.Sample {
		if sm.ID == "b1" && len(sm.Diffs) == 1 {
			diff = &sm.Diffs[0]
		}
	}
	if diff == nil || diff.Field != "Publisher" || diff.Before != "DC" || diff.After != "DC Comics" {
		t.Errorf("diff = %+v", diff)
	}
	if b, _ := s.backend.GetBook("b1"); b.Publisher != "DC" || b.ComicBookIsDirty {
		t.Errorf("preview wrote: %q dirty=%v", b.Publisher, b.ComicBookIsDirty)
	}
	if rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{Limit: 10}); len(rows) != 0 {
		t.Errorf("preview was audited: %d rows", len(rows))
	}

	if code, _, raw = postBulkEdit(t, s, `{"book_ids":["b1","b2","b3"],"expected_count":3,"apply":true,`+edit+`}`); code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, raw)
	}
	b2, _ := s.backend.GetBook("b2")
	if b2.Publisher != "DC Comics Comics" || !b2.ComicBookIsDirty {
		t.Errorf("b2 = %q dirty=%v", b2.Publisher, b2.ComicBookIsDirty)
	}
	if rows, _, _ := s.configDB.ListAudit(configdb.AuditFilter{Limit: 10}); len(rows) != 3 {
		t.Errorf("audit rows = %d, want 3", len(rows))
	}
	// A stale expected_count is still refused.
	if code, _, _ = postBulkEdit(t, s, `{"book_ids":["b1"],"expected_count":2,"apply":true,`+edit+`}`); code != http.StatusConflict {
		t.Errorf("stale count: %d", code)
	}
}

func TestBulkEdit_FindReplaceRules(t *testing.T) {
	s := newDuplicatesTestServer(t, t.TempDir(), bulkEditBooks())
	for name, edit := range map[string]string{
		"empty find":        `{"find_replace":{"field":"Series","find":"","replace":"x"}}`,
		"custom value":      `{"find_replace":{"field":"comic_server_hash","find":"a","replace":"x"}}`,
		"numeric field":     `{"find_replace":{"field":"Year","find":"1","replace":"2"}}`,
		"not editable":      `{"find_replace":{"field":"FilePath","find":"a","replace":"x"}}`,
		"also set directly": `{"fields":{"Series":"Z"},"find_replace":{"field":"Series","find":"a","replace":"x"}}`,
		"also list add":     `{"list_add":{"Tags":"q"},"find_replace":{"field":"Tags","find":"a","replace":"x"}}`,
	} {
		if code, _, raw := postBulkEdit(t, s, `{"book_ids":["b1"],"edit":`+edit+`}`); code != http.StatusBadRequest {
			t.Errorf("%s: status %d %s", name, code, raw)
		}
	}
	// A list field is replaced as its whole comma-separated text.
	code, _, raw := postBulkEdit(t, s, `{"book_ids":["b2"],"expected_count":1,"apply":true,"edit":{"find_replace":{"field":"Tags","find":"b","replace":"c"}}}`)
	if code != http.StatusOK {
		t.Fatalf("list field: %d %s", code, raw)
	}
	if b, _ := s.backend.GetBook("b2"); b.Tags != "a, c" {
		t.Errorf("Tags = %q", b.Tags)
	}
	// No match changes nothing.
	_, resp, _ := postBulkEdit(t, s, `{"book_ids":["b1"],"edit":{"find_replace":{"field":"Series","find":"zzz","replace":"x"}}}`)
	if resp.Changed != 0 || resp.Unchanged != 1 {
		t.Errorf("no-match resp = %+v", resp)
	}
}
