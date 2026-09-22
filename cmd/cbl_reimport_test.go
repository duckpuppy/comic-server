package cmd

import "testing"

func TestParseGitCBLSourceCLI(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		wantOK      bool
		wantRepoURL string
		wantPath    string
	}{
		{
			name:        "git source",
			source:      "git:https://github.com/DieselTech/CBL-ReadingLists:Marvel/Batman.cbl",
			wantOK:      true,
			wantRepoURL: "https://github.com/DieselTech/CBL-ReadingLists",
			wantPath:    "Marvel/Batman.cbl",
		},
		{
			name:   "local file source",
			source: "local_file",
			wantOK: false,
		},
		{
			name:   "empty source",
			source: "",
			wantOK: false,
		},
		{
			name:   "git prefix with no path separator",
			source: "git:no-colon-here",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoURL, path, ok := parseGitCBLSourceCLI(tt.source)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if repoURL != tt.wantRepoURL {
				t.Errorf("repoURL = %q, want %q", repoURL, tt.wantRepoURL)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}
