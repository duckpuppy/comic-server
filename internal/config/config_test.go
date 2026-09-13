package config

import (
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg == nil {
		t.Fatal("NewConfig() returned nil")
	}

	if cfg.Devices == nil {
		t.Error("NewConfig() Devices map is nil")
	}

	if len(cfg.Devices) != 0 {
		t.Errorf("NewConfig() Devices map not empty, got %d devices", len(cfg.Devices))
	}
}

func TestKomgaConfigValidate(t *testing.T) {
	validTarget := KomgaTarget{
		ListID:    "{GUID-1}",
		Type:      KomgaTargetCollection,
		KomgaName: "My Collection",
		Enabled:   true,
	}

	tests := []struct {
		name        string
		komga       KomgaConfig
		libraryRoot string
		wantErr     bool
	}{
		{
			name:    "disabled config is always valid",
			komga:   KomgaConfig{Enabled: false},
			wantErr: false,
		},
		{
			name: "enabled with all required fields",
			komga: KomgaConfig{
				Enabled:    true,
				BaseURL:    "https://komga.example.com",
				APIKey:     "secret",
				RemoteRoot: "/mnt/zfs/comics",
				Targets:    []KomgaTarget{validTarget},
			},
			libraryRoot: "/comics",
			wantErr:     false,
		},
		{
			name:        "enabled without base_url",
			komga:       KomgaConfig{Enabled: true, APIKey: "secret", RemoteRoot: "b"},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name:        "enabled without api_key",
			komga:       KomgaConfig{Enabled: true, BaseURL: "https://x", RemoteRoot: "b"},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name:    "enabled without library_root",
			komga:   KomgaConfig{Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b"},
			wantErr: true,
		},
		{
			name:        "enabled without remote_root",
			komga:       KomgaConfig{Enabled: true, BaseURL: "https://x", APIKey: "secret"},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name: "target missing list_id",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b",
				Targets: []KomgaTarget{{Type: KomgaTargetCollection, KomgaName: "X"}},
			},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name: "target missing komga_name",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b",
				Targets: []KomgaTarget{{ListID: "{GUID-1}", Type: KomgaTargetCollection}},
			},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name: "target invalid type",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b",
				Targets: []KomgaTarget{{ListID: "{GUID-1}", Type: "bogus", KomgaName: "X"}},
			},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name: "duplicate list_id across targets",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b",
				Targets: []KomgaTarget{
					{ListID: "{GUID-1}", Type: KomgaTargetCollection, KomgaName: "A"},
					{ListID: "{GUID-1}", Type: KomgaTargetReadList, KomgaName: "B"},
				},
			},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name: "negative sync_interval_sec",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b",
				SyncIntervalSec: -1,
			},
			libraryRoot: "a",
			wantErr:     true,
		},
		{
			name: "readlist target type is valid",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b",
				Targets: []KomgaTarget{{ListID: "{GUID-1}", Type: KomgaTargetReadList, KomgaName: "X"}},
			},
			libraryRoot: "a",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.komga.Validate(tt.libraryRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCBZConvertConfig_ValidateRequiresTrashPath is a direct unit test on
// CBZConvertConfig.Validate itself, not through Config.Validate() -
// comic-server-dtu5 moved the actual call site to cmd/server.go's
// validateCBZConvertAgainstEffectiveTrash, which runs after config.db
// opens (checking config.db's trash_settings first, config.yaml's
// TrashPath as fallback) rather than as part of Config.Validate(), since
// config.db isn't open yet at that point in startup.
func TestCBZConvertConfig_ValidateRequiresTrashPath(t *testing.T) {
	cc := CBZConvertConfig{Enabled: true}

	if err := cc.Validate(""); err == nil {
		t.Error("Validate() should reject cbz_convert.enabled without a trash path")
	}
	if err := cc.Validate("/data/trash"); err != nil {
		t.Errorf("Validate() should accept cbz_convert.enabled with a trash path set, got: %v", err)
	}
}

func TestCBZConvertConfig_ValidateDisabledDoesNotRequireTrashPath(t *testing.T) {
	cc := CBZConvertConfig{Enabled: false}

	if err := cc.Validate(""); err != nil {
		t.Errorf("Validate() should not require a trash path when disabled, got: %v", err)
	}
}

// TestConfigValidate_DoesNotCheckCBZConvert confirms Config.Validate()
// itself no longer touches CBZConvert at all (comic-server-dtu5) - it
// can't, since config.db (where the effective trash path might actually
// live) isn't open yet when Validate() runs.
func TestConfigValidate_DoesNotCheckCBZConvert(t *testing.T) {
	cfg := NewConfig()
	cfg.ApplyDefaults()
	cfg.Server.CBZConvert.Enabled = true
	cfg.Server.TrashPath = ""

	if err := cfg.Validate(); err != nil {
		t.Errorf("Config.Validate() should not itself validate CBZConvert (moved to cmd/server.go), got: %v", err)
	}
}

func TestConfigValidate_KomgaWiredIn(t *testing.T) {
	cfg := NewConfig()
	cfg.Server.Komga = KomgaConfig{Enabled: true} // missing everything else

	if err := cfg.Validate(); err == nil {
		t.Error("Config.Validate() should surface Komga validation errors")
	}
}

func TestApplyDefaults_TrashRetentionDays(t *testing.T) {
	cfg := NewConfig()
	cfg.ApplyDefaults()

	if cfg.Server.TrashRetentionDays != 30 {
		t.Errorf("TrashRetentionDays default = %d, want 30", cfg.Server.TrashRetentionDays)
	}
}

func TestConfigValidate_TrashRetentionDaysNegative(t *testing.T) {
	cfg := NewConfig()
	cfg.ApplyDefaults()
	cfg.Server.TrashRetentionDays = -1

	if err := cfg.Validate(); err == nil {
		t.Error("Config.Validate() should reject a negative trash_retention_days")
	}
}
