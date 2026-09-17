package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleBrowseDirectory_ListsSubdirectoriesOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "b-folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "a-folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "not-a-dir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/browse-directory?path="+root, nil)
	w := httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DirectoryBrowseResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Directories) != 2 || resp.Directories[0] != "a-folder" || resp.Directories[1] != "b-folder" {
		t.Errorf("Directories = %v, want [a-folder b-folder] (sorted, no files, no dotdirs)", resp.Directories)
	}
	if resp.Parent != filepath.Dir(root) {
		t.Errorf("Parent = %q, want %q", resp.Parent, filepath.Dir(root))
	}
}

// TestHandleBrowseDirectory_FilesOnlyIncludedWithQueryParam covers
// comic-server-38f7's extension of the Watch Folders directory browser
// (comic-server-obe) into a file picker: files are listed only when
// ?files=1 is passed (Watch Folders itself never sets it, so its
// behavior is unaffected), and only recognized comic archive extensions
// are returned, not arbitrary files.
func TestHandleBrowseDirectory_FilesOnlyIncludedWithQueryParam(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "book.cbz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/system/browse-directory?path="+root, nil)
	w := httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)
	var withoutFiles DirectoryBrowseResponse
	json.NewDecoder(w.Body).Decode(&withoutFiles)
	if len(withoutFiles.Files) != 0 {
		t.Errorf("expected no files without ?files=1, got %v", withoutFiles.Files)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/system/browse-directory?path="+root+"&files=1", nil)
	w = httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)
	var withFiles DirectoryBrowseResponse
	json.NewDecoder(w.Body).Decode(&withFiles)
	if len(withFiles.Files) != 1 || withFiles.Files[0] != "book.cbz" {
		t.Errorf("Files = %v, want [book.cbz] (notes.txt is not a comic archive)", withFiles.Files)
	}
}

func TestHandleBrowseDirectory_EmptyPathDefaultsToRoot(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/browse-directory", nil)
	w := httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DirectoryBrowseResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Path != "/" {
		t.Errorf("Path = %q, want \"/\"", resp.Path)
	}
	if resp.Parent != "" {
		t.Errorf("Parent = %q, want empty at the filesystem root", resp.Parent)
	}
}

func TestHandleBrowseDirectory_NonexistentPathIs400(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/browse-directory?path=/does/not/exist/at/all", nil)
	w := httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBrowseDirectory_FilePathIs400(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/browse-directory?path="+filePath, nil)
	w := httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a file path, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleBrowseDirectory_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/system/browse-directory", nil)
	w := httptest.NewRecorder()
	s.handleBrowseDirectory(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
