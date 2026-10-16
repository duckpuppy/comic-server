package comicvine

import "testing"

func TestParseCVRef(t *testing.T) {
	ok := []struct {
		in   string
		kind string
		id   int
	}{
		{"https://comicvine.gamespot.com/godzilla-here-there-be-aliens-1/4000-1034567/", "issue", 1034567},
		{"https://comicvine.gamespot.com/godzilla/4050-98765/", "volume", 98765},
		{"  4000-123 ", "issue", 123},
		{"cv:4050-77", "volume", 77},
		{"123456", "issue", 123456},
		{"https://comicvine.gamespot.com/issue/4000-55/?x=1", "issue", 55},
	}
	for _, c := range ok {
		got, err := ParseCVRef(c.in)
		if err != nil || got.Kind != c.kind || got.ID != c.id {
			t.Errorf("%q = %+v, %v; want %s %d", c.in, got, err, c.kind, c.id)
		}
	}
	bad := []string{"", "hello", "0", "https://example.com/x/4000-5/", "https://comicvine.gamespot.com/batman/4005-1699/", "4999-12", "https://evil.test/comicvine.gamespot.com/4000-1/"}
	for _, in := range bad {
		if got, err := ParseCVRef(in); err == nil {
			t.Errorf("%q accepted as %+v", in, got)
		}
	}
}
