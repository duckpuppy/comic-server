package comicvine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSniffArchiveAndDispatch(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	rar := write("really-rar.cbz", []byte("Rar!\x1a\x07\x01\x00rest"))
	zip := write("really-zip.cbr", []byte("PK\x03\x04rest"))
	sz := write("really-7z.cbz", []byte("7z\xbc\xaf\x27\x1crest"))
	cbwZip := write("webcomic.cbw", []byte("PK\x03\x04rest"))
	cbw7z := write("webcomic2.cbw", []byte("7z\xbc\xaf\x27\x1crest"))
	oddJunk := write("notes.txt", []byte("hello"))
	junk := write("junk.cbz", []byte("hello"))
	empty := write("empty.cbz", nil)

	tests := []struct {
		path     string
		kind     ArchiveKind
		dispatch string
	}{
		{rar, KindRAR, ".cbr"},
		{zip, KindZIP, ".cbz"},
		{sz, Kind7Z, ".cb7"},
		{cbwZip, KindZIP, ".cbz"},
		{cbw7z, Kind7Z, ".cb7"},
		{oddJunk, KindUnknown, ".txt"}, // unknown ext, unknown content: unsupported
		{junk, KindUnknown, ".cbz"},    // unknown content: fall back to extension
		{empty, KindUnknown, ".cbz"},
	}
	for _, tt := range tests {
		got, err := SniffArchive(tt.path)
		if err != nil || got != tt.kind {
			t.Errorf("SniffArchive(%s) = %q, %v; want %q", filepath.Base(tt.path), got, err, tt.kind)
		}
		if d := dispatchExt(tt.path); d != tt.dispatch {
			t.Errorf("dispatchExt(%s) = %q, want %q", filepath.Base(tt.path), d, tt.dispatch)
		}
	}
	if _, err := SniffArchive(filepath.Join(dir, "nope.cbz")); err == nil {
		t.Error("missing file should error")
	}
}
