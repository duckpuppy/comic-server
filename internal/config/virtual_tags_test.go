package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_VirtualTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := `server:
  virtual_tags:
    - id: 1
      name: Series and volume
      caption_format: "{Series}[ v{Volume}]"
    - id: 5
      name: Off for now
      caption_format: "{Publisher}"
      enabled: false
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tags := cfg.Server.VirtualTags
	if len(tags) != 2 {
		t.Fatalf("got %d tags, want 2", len(tags))
	}
	if tags[0].ID != 1 || tags[0].Name != "Series and volume" || tags[0].CaptionFormat != "{Series}[ v{Volume}]" {
		t.Errorf("tag 0 = %+v", tags[0])
	}
	if !tags[0].IsEnabled() {
		t.Error("tag with enabled omitted should default to enabled")
	}
	if tags[1].IsEnabled() {
		t.Error("tag with enabled: false should be disabled")
	}
}

func TestValidateVirtualTags(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name    string
		tags    []VirtualTagConfig
		wantErr string
	}{
		{"none", nil, ""},
		{"valid", []VirtualTagConfig{{ID: 1, Name: "A", CaptionFormat: "{Series}"}, {ID: 20, Name: "B", CaptionFormat: "x", Enabled: &yes}}, ""},
		{"id too low", []VirtualTagConfig{{ID: 0, Name: "A", CaptionFormat: "x"}}, "id must be between"},
		{"id too high", []VirtualTagConfig{{ID: 21, Name: "A", CaptionFormat: "x"}}, "id must be between"},
		{"duplicate id", []VirtualTagConfig{{ID: 2, Name: "A", CaptionFormat: "x"}, {ID: 2, Name: "B", CaptionFormat: "y"}}, "duplicate id 2"},
		{"enabled needs name", []VirtualTagConfig{{ID: 1, CaptionFormat: "x"}}, "name is required"},
		{"enabled needs format", []VirtualTagConfig{{ID: 1, Name: "A"}}, "caption_format is required"},
		{"disabled may be incomplete", []VirtualTagConfig{{ID: 1, Enabled: &no}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVirtualTags(tt.tags)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidate_RejectsBadVirtualTags(t *testing.T) {
	cfg := NewConfig()
	cfg.ApplyDefaults()
	cfg.Server.VirtualTags = []VirtualTagConfig{{ID: 99, Name: "A", CaptionFormat: "x"}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "virtual_tags") {
		t.Errorf("Validate() = %v, want a virtual_tags error", err)
	}
}
