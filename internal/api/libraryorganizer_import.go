package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/libraryorganizer"
	"github.com/google/uuid"
)

// Import of a ComicRack Library Organizer losettingsx.dat from the web UI
// (tracker #71). Two stateless steps: import-preview parses the uploaded file
// and writes nothing; import takes the same file again plus the user's
// choices (which profiles, edited names and paths).

// maxLOImportUploadBytes caps the upload. A real file is about 12 KB.
const maxLOImportUploadBytes = 4 << 20

// LOImportPreviewProfile is one profile found in the uploaded file.
type LOImportPreviewProfile struct {
	Index               int    `json:"index"`
	Name                string `json:"name"`
	Mode                string `json:"mode"`
	CopyMode            bool   `json:"copy_mode"`
	FolderTemplate      string `json:"folder_template"`
	FileTemplate        string `json:"file_template"`
	BaseFolder          string `json:"base_folder"`           // as written in the file
	SuggestedBaseFolder string `json:"suggested_base_folder"` // after the server's library path mapping
	MoveFailed          bool   `json:"move_failed"`
	FailedFolder        string `json:"failed_folder"`
	SuggestedFailedDir  string `json:"suggested_failed_folder"`
	ItemCount           int    `json:"item_count"`
	ExcludeRuleCount    int    `json:"exclude_rule_count"`
	ExistingProfileID   string `json:"existing_profile_id,omitempty"` // a profile with the same name is already here
	ExistingProfileName string `json:"existing_profile_name,omitempty"`
}

// LOImportPreview is the response of POST .../organize-profiles/import-preview.
type LOImportPreview struct {
	LastUsed    string                   `json:"last_used"`
	LibraryRoot string                   `json:"library_root"`
	Profiles    []LOImportPreviewProfile `json:"profiles"`
}

// LOImportChoice is the user's decision for one profile in the file.
type LOImportChoice struct {
	Index        int    `json:"index"`
	Import       bool   `json:"import"`
	Name         string `json:"name"`
	BaseFolder   string `json:"base_folder"`
	FailedFolder string `json:"failed_folder"`
	Replace      bool   `json:"replace"` // replace an existing profile with the same name
}

// LOImportResult is the response of POST .../organize-profiles/import.
type LOImportResult struct {
	Imported int `json:"imported"`
	Replaced int `json:"replaced"`
}

// readLOUpload reads and parses the multipart "file" field.
func (s *Server) readLOUpload(w http.ResponseWriter, r *http.Request) (*libraryorganizer.ImportResult, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLOImportUploadBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "Failed to read the upload: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `Missing "file" upload field`, http.StatusBadRequest)
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read the upload: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}
	res, err := libraryorganizer.ParseLOSettings(bytes.NewReader(data), func() string { return uuid.New().String() })
	if err != nil || len(res.Profiles) == 0 {
		msg := "This does not look like a Library Organizer losettingsx.dat file."
		if err == nil {
			msg = "The file has no profiles."
		}
		http.Error(w, msg, http.StatusBadRequest)
		return nil, false
	}
	return res, true
}

// resolveImportPath applies the server's library path mapping (Server.
// LibrarySourceRoot to LibraryRoot) to a path from the file. Without that
// mapping it returns the path unchanged, and the user edits it by hand.
func (s *Server) resolveImportPath(raw string) string {
	if raw == "" {
		return ""
	}
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	if s.config == nil {
		return raw
	}
	return s.config.ResolveLibraryFilePath(raw)
}

// handleLOImportPreview parses an uploaded losettingsx.dat and reports what
// it holds. It writes nothing.
// POST /api/library/organize-profiles/import-preview
func (s *Server) handleLOImportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, ok := s.readLOUpload(w, r)
	if !ok {
		return
	}
	existing, err := s.configDB.ListLOProfiles()
	if err != nil {
		http.Error(w, "Failed to list profiles", http.StatusInternalServerError)
		return
	}
	byName := make(map[string]configdb.LOProfile, len(existing))
	for _, p := range existing {
		byName[strings.ToLower(strings.TrimSpace(p.Name))] = p
	}

	out := LOImportPreview{LastUsed: res.LastUsed}
	s.configMu.RLock()
	if s.config != nil {
		out.LibraryRoot = s.config.Server.LibraryRoot
	}
	s.configMu.RUnlock()

	for i, p := range res.Profiles {
		pp := LOImportPreviewProfile{
			Index:               i,
			Name:                p.Name,
			Mode:                p.Mode,
			CopyMode:            p.CopyMode,
			FolderTemplate:      p.FolderTemplate,
			FileTemplate:        p.FileTemplate,
			BaseFolder:          p.BaseFolder,
			SuggestedBaseFolder: s.resolveImportPath(p.BaseFolder),
			MoveFailed:          p.MoveFailed,
			FailedFolder:        p.FailedFolder,
			SuggestedFailedDir:  s.resolveImportPath(p.FailedFolder),
			ItemCount:           len(p.Items),
			ExcludeRuleCount:    len(p.ExcludeRules),
		}
		if ex, found := byName[strings.ToLower(strings.TrimSpace(p.Name))]; found {
			pp.ExistingProfileID = ex.ID
			pp.ExistingProfileName = ex.Name
		}
		out.Profiles = append(out.Profiles, pp)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleLOImport adds the chosen profiles from an uploaded losettingsx.dat.
// The form has the "file" field and a "choices" field with a JSON array of
// LOImportChoice. Other profiles are never touched, except one the user
// chose to replace.
// POST /api/library/organize-profiles/import
func (s *Server) handleLOImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, ok := s.readLOUpload(w, r)
	if !ok {
		return
	}
	var choices []LOImportChoice
	if err := json.Unmarshal([]byte(r.FormValue("choices")), &choices); err != nil {
		http.Error(w, `Missing or invalid "choices" field`, http.StatusBadRequest)
		return
	}

	existing, err := s.configDB.ListLOProfiles()
	if err != nil {
		http.Error(w, "Failed to list profiles", http.StatusInternalServerError)
		return
	}
	byName := make(map[string]configdb.LOProfile, len(existing))
	nextSort := 0
	for _, p := range existing {
		byName[strings.ToLower(strings.TrimSpace(p.Name))] = p
		if p.SortOrder >= nextSort {
			nextSort = p.SortOrder + 1
		}
	}

	var (
		toAdd      []configdb.LOImportProfile
		replaceIDs []string
		seenIndex  = map[int]bool{}
		seenName   = map[string]bool{}
	)
	for _, c := range choices {
		if !c.Import {
			continue
		}
		if c.Index < 0 || c.Index >= len(res.Profiles) || seenIndex[c.Index] {
			http.Error(w, fmt.Sprintf("Profile index %d is not in the file", c.Index), http.StatusBadRequest)
			return
		}
		seenIndex[c.Index] = true

		name := strings.TrimSpace(c.Name)
		if name == "" {
			http.Error(w, "Every imported profile needs a name", http.StatusBadRequest)
			return
		}
		key := strings.ToLower(name)
		if seenName[key] {
			http.Error(w, fmt.Sprintf("Two imported profiles are both named %q", name), http.StatusBadRequest)
			return
		}
		seenName[key] = true
		if ex, found := byName[key]; found {
			if !c.Replace {
				http.Error(w, fmt.Sprintf("A profile named %q already exists. Rename the import or choose to replace it.", ex.Name), http.StatusConflict)
				return
			}
			replaceIDs = append(replaceIDs, ex.ID)
		}

		p := toLOImportProfile(res.Profiles[c.Index])
		p.Name = name
		p.BaseFolder = strings.TrimSpace(c.BaseFolder)
		p.FailedFolder = strings.TrimSpace(c.FailedFolder)
		p.SortOrder = nextSort
		nextSort++
		toAdd = append(toAdd, p)
	}
	if len(toAdd) == 0 {
		http.Error(w, "No profiles were chosen to import", http.StatusBadRequest)
		return
	}

	if err := s.configDB.AddLibraryOrganizerProfiles(toAdd, replaceIDs); err != nil {
		http.Error(w, "Failed to import profiles: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, LOImportResult{Imported: len(toAdd), Replaced: len(replaceIDs)})
}

// toLOImportProfile converts a parsed profile to the storage shape.
func toLOImportProfile(p libraryorganizer.ImportedProfile) configdb.LOImportProfile {
	items := make([]configdb.LOImportItem, len(p.Items))
	for i, it := range p.Items {
		items[i] = configdb.LOImportItem{Category: it.Category, Name: it.Name, Value: it.Value}
	}
	rules := make([]configdb.LOImportExcludeRule, len(p.ExcludeRules))
	for i, r := range p.ExcludeRules {
		rules[i] = configdb.LOImportExcludeRule{Field: r.Field, Operator: r.Operator, Value: r.Value}
	}
	return configdb.LOImportProfile{
		ID:                    p.ID,
		Name:                  p.Name,
		BaseFolder:            p.BaseFolder,
		FolderTemplate:        p.FolderTemplate,
		FileTemplate:          p.FileTemplate,
		EmptyFolder:           p.EmptyFolder,
		Mode:                  p.Mode,
		CopyMode:              p.CopyMode,
		UseFolder:             p.UseFolder,
		UseFileName:           p.UseFileName,
		ReplaceMultipleSpaces: p.ReplaceMultipleSpaces,
		AutoSpaceFields:       p.AutoSpaceFields,
		RemoveEmptyFolder:     p.RemoveEmptyFolder,
		MoveFileless:          p.MoveFileless,
		FilelessFormat:        p.FilelessFormat,
		FailEmptyValues:       p.FailEmptyValues,
		MoveFailed:            p.MoveFailed,
		FailedFolder:          p.FailedFolder,
		ExcludeMode:           p.ExcludeMode,
		ExcludeOperator:       p.ExcludeOperator,
		SortOrder:             p.SortOrder,
		Items:                 items,
		ExcludeRules:          rules,
	}
}
