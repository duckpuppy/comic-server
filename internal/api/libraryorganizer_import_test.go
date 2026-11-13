package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duckpuppy/comic-server/internal/config"
)

const loImportSample = `<?xml version="1.0" encoding="utf-8"?>
<Profiles LastUsed="Default">
  <Profile Name="Default" Version="2.1">
    <FolderTemplate>{&lt;publisher&gt;}\{&lt;series&gt;}</FolderTemplate>
    <BaseFolder>G:\Comics</BaseFolder>
    <FileTemplate>{&lt;series&gt;}{ #&lt;number2&gt;}</FileTemplate>
    <Postfix><Item Name="Format" Value=")" /></Postfix>
    <Months><Item Name="1" Value="January" /></Months>
    <UseFolder>true</UseFolder><UseFileName>true</UseFileName>
    <Mode>Move</Mode>
  </Profile>
  <Profile Name="Archive" Version="2.1">
    <FolderTemplate>{&lt;publisher&gt;}</FolderTemplate>
    <BaseFolder>G:\Archive</BaseFolder>
    <FileTemplate />
    <Mode>Move</Mode><CopyMode>true</CopyMode>
  </Profile>
</Profiles>`

func loUploadRequest(t *testing.T, path, fileBody, choices string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "losettingsx.dat")
	_, _ = fw.Write([]byte(fileBody))
	if choices != "" {
		_ = mw.WriteField("choices", choices)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestLOImportPreview_ListsProfilesAndMapsPaths(t *testing.T) {
	s := newLOProfilesTestServer(t)
	s.config = &config.Config{Server: config.ServerConfig{LibrarySourceRoot: `G:\Comics`, LibraryRoot: "/data/comics"}}

	w := httptest.NewRecorder()
	s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import-preview", loImportSample, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got LOImportPreview
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.LastUsed != "Default" || len(got.Profiles) != 2 || got.LibraryRoot != "/data/comics" {
		t.Fatalf("preview = %+v", got)
	}
	d := got.Profiles[0]
	if d.Name != "Default" || d.BaseFolder != `G:\Comics` || d.SuggestedBaseFolder != "/data/comics" {
		t.Errorf("Default profile: %+v", d)
	}
	if d.ItemCount != 2 {
		t.Errorf("item count = %d, want 2", d.ItemCount)
	}
	// A path outside the mapped root stays as written, for the user to edit.
	if got.Profiles[1].SuggestedBaseFolder != `G:\Archive` {
		t.Errorf("unmapped path changed: %q", got.Profiles[1].SuggestedBaseFolder)
	}
	// Nothing was written.
	list, _ := s.configDB.ListLOProfiles()
	if len(list) != 0 {
		t.Errorf("preview wrote %d profiles", len(list))
	}
}

func TestLOImportPreview_RejectsNonProfileFile(t *testing.T) {
	s := newLOProfilesTestServer(t)
	for _, body := range []string{"not xml at all", "<Profiles LastUsed=\"x\"></Profiles>"} {
		w := httptest.NewRecorder()
		s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import-preview", body, ""))
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", body, w.Code)
		}
	}
}

func TestLOImport_AddsChosenProfilesWithEditedPaths(t *testing.T) {
	s := newLOProfilesTestServer(t)
	choices, _ := json.Marshal([]LOImportChoice{
		{Index: 0, Import: true, Name: "Default", BaseFolder: "/data/comics"},
		{Index: 1, Import: false},
	})
	w := httptest.NewRecorder()
	s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import", loImportSample, string(choices)))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	list, _ := s.configDB.ListLOProfiles()
	if len(list) != 1 || list[0].Name != "Default" || list[0].BaseFolder != "/data/comics" {
		t.Fatalf("profiles = %+v", list)
	}
	if list[0].FolderTemplate != `{<publisher>}\{<series>}` {
		t.Errorf("template = %q", list[0].FolderTemplate)
	}
	// The item lists came along.
	months, _ := s.configDB.ListLOProfileItems(list[0].ID, "months")
	if len(months) != 1 || months[0].Value != "January" {
		t.Errorf("months = %+v", months)
	}
}

func TestLOImport_SameNameNeedsReplaceOrRename(t *testing.T) {
	s := newLOProfilesTestServer(t)
	first, _ := json.Marshal([]LOImportChoice{{Index: 0, Import: true, Name: "Default", BaseFolder: "/a"}})
	w := httptest.NewRecorder()
	s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import", loImportSample, string(first)))
	if w.Code != http.StatusOK {
		t.Fatalf("first import: %d %s", w.Code, w.Body.String())
	}
	before, _ := s.configDB.ListLOProfiles()

	// Same name, no replace: refused, nothing changed.
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import", loImportSample, string(first)))
	if w.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", w.Code)
	}
	after, _ := s.configDB.ListLOProfiles()
	if len(after) != len(before) {
		t.Fatal("a refused import changed the profiles")
	}

	// Renamed: both exist.
	renamed, _ := json.Marshal([]LOImportChoice{{Index: 0, Import: true, Name: "Default 2", BaseFolder: "/b"}})
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import", loImportSample, string(renamed)))
	if w.Code != http.StatusOK {
		t.Fatalf("renamed import: %d %s", w.Code, w.Body.String())
	}
	if list, _ := s.configDB.ListLOProfiles(); len(list) != 2 {
		t.Fatalf("profiles = %d, want 2", len(list))
	}

	// Replace (case-insensitive name match): the old one is swapped out.
	repl, _ := json.Marshal([]LOImportChoice{{Index: 0, Import: true, Name: "default", BaseFolder: "/c", Replace: true}})
	w = httptest.NewRecorder()
	s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import", loImportSample, string(repl)))
	if w.Code != http.StatusOK {
		t.Fatalf("replace import: %d %s", w.Code, w.Body.String())
	}
	var res LOImportResult
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Imported != 1 || res.Replaced != 1 {
		t.Errorf("result = %+v", res)
	}
	list, _ := s.configDB.ListLOProfiles()
	if len(list) != 2 {
		t.Fatalf("profiles after replace = %d, want 2", len(list))
	}
	for _, p := range list {
		if strings.EqualFold(p.Name, "default") && p.BaseFolder != "/c" {
			t.Errorf("replaced profile has base %q, want /c", p.BaseFolder)
		}
	}
}

func TestLOImport_Validation(t *testing.T) {
	s := newLOProfilesTestServer(t)
	cases := map[string][]LOImportChoice{
		"nothing chosen":  {{Index: 0, Import: false}},
		"blank name":      {{Index: 0, Import: true, Name: "  "}},
		"bad index":       {{Index: 9, Import: true, Name: "X"}},
		"duplicate index": {{Index: 0, Import: true, Name: "A"}, {Index: 0, Import: true, Name: "B"}},
		"duplicate names": {{Index: 0, Import: true, Name: "A"}, {Index: 1, Import: true, Name: "a"}},
	}
	for name, c := range cases {
		body, _ := json.Marshal(c)
		w := httptest.NewRecorder()
		s.handleLOProfilesRouter(w, loUploadRequest(t, "/api/library/organize-profiles/import", loImportSample, string(body)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, w.Code, w.Body.String())
		}
	}
	if list, _ := s.configDB.ListLOProfiles(); len(list) != 0 {
		t.Errorf("a refused import left %d profiles", len(list))
	}
}
