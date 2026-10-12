package comicvine

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveKind is the real container format of an archive file, by content.
type ArchiveKind string

const (
	KindUnknown ArchiveKind = ""
	KindZIP     ArchiveKind = "zip"
	KindRAR     ArchiveKind = "rar"
	Kind7Z      ArchiveKind = "7z"
)

// SniffArchive reads path's first bytes and reports the real container:
// a RAR saved as .cbz is a RAR (comic-server-0ebv). KindUnknown means no
// known signature - the file may be empty, truncated or not an archive.
func SniffArchive(path string) (ArchiveKind, error) {
	f, err := os.Open(path)
	if err != nil {
		return KindUnknown, err
	}
	defer f.Close()
	head := make([]byte, 8)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return KindUnknown, err
	}
	return kindFromHeader(head[:n]), nil
}

func kindFromHeader(h []byte) ArchiveKind {
	switch {
	case bytes.HasPrefix(h, []byte("PK\x03\x04")), // normal
		bytes.HasPrefix(h, []byte("PK\x05\x06")), // empty
		bytes.HasPrefix(h, []byte("PK\x07\x08")): // spanned
		return KindZIP
	case bytes.HasPrefix(h, []byte("Rar!\x1a\x07")):
		return KindRAR
	case bytes.HasPrefix(h, []byte("7z\xbc\xaf\x27\x1c")):
		return Kind7Z
	}
	return KindUnknown
}

// ExtKind is the container an extension claims, or KindUnknown.
func ExtKind(path string) ArchiveKind {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cbz", ".zip":
		return KindZIP
	case ".cbr", ".rar":
		return KindRAR
	case ".cb7", ".7z":
		return Kind7Z
	}
	return KindUnknown
}

// dispatchExt is the extension the format switches should use: the real
// container's canonical extension when the content is recognisable,
// otherwise the file's own lower-cased extension. A misnamed archive is
// thus read as what it really is, including one with an extension we do
// not know at all (.cbw and friends are just renamed cbz/cbr/cb7).
func dispatchExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	kind, err := SniffArchive(path)
	if err != nil {
		return ext
	}
	switch kind {
	case KindZIP:
		return ".cbz"
	case KindRAR:
		return ".cbr"
	case Kind7Z:
		return ".cb7"
	}
	return ext
}
