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
		name    string
		komga   KomgaConfig
		wantErr bool
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
				LocalRoot:  `G:\Comics\`,
				RemoteRoot: "/mnt/zfs/comics",
				Targets:    []KomgaTarget{validTarget},
			},
			wantErr: false,
		},
		{
			name:    "enabled without base_url",
			komga:   KomgaConfig{Enabled: true, APIKey: "secret", LocalRoot: "a", RemoteRoot: "b"},
			wantErr: true,
		},
		{
			name:    "enabled without api_key",
			komga:   KomgaConfig{Enabled: true, BaseURL: "https://x", LocalRoot: "a", RemoteRoot: "b"},
			wantErr: true,
		},
		{
			name:    "enabled without local_root",
			komga:   KomgaConfig{Enabled: true, BaseURL: "https://x", APIKey: "secret", RemoteRoot: "b"},
			wantErr: true,
		},
		{
			name:    "enabled without remote_root",
			komga:   KomgaConfig{Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a"},
			wantErr: true,
		},
		{
			name: "target missing list_id",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a", RemoteRoot: "b",
				Targets: []KomgaTarget{{Type: KomgaTargetCollection, KomgaName: "X"}},
			},
			wantErr: true,
		},
		{
			name: "target missing komga_name",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a", RemoteRoot: "b",
				Targets: []KomgaTarget{{ListID: "{GUID-1}", Type: KomgaTargetCollection}},
			},
			wantErr: true,
		},
		{
			name: "target invalid type",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a", RemoteRoot: "b",
				Targets: []KomgaTarget{{ListID: "{GUID-1}", Type: "bogus", KomgaName: "X"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate list_id across targets",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a", RemoteRoot: "b",
				Targets: []KomgaTarget{
					{ListID: "{GUID-1}", Type: KomgaTargetCollection, KomgaName: "A"},
					{ListID: "{GUID-1}", Type: KomgaTargetReadList, KomgaName: "B"},
				},
			},
			wantErr: true,
		},
		{
			name: "negative sync_interval_sec",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a", RemoteRoot: "b",
				SyncIntervalSec: -1,
			},
			wantErr: true,
		},
		{
			name: "readlist target type is valid",
			komga: KomgaConfig{
				Enabled: true, BaseURL: "https://x", APIKey: "secret", LocalRoot: "a", RemoteRoot: "b",
				Targets: []KomgaTarget{{ListID: "{GUID-1}", Type: KomgaTargetReadList, KomgaName: "X"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.komga.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidate_CBZConvertRequiresTrashPath(t *testing.T) {
	cfg := NewConfig()
	cfg.ApplyDefaults()
	cfg.Server.CBZConvert.Enabled = true
	cfg.Server.TrashPath = ""

	if err := cfg.Validate(); err == nil {
		t.Error("Config.Validate() should reject cbz_convert.enabled without trash_path set")
	}

	cfg.Server.TrashPath = "/data/trash"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Config.Validate() should accept cbz_convert.enabled with trash_path set, got: %v", err)
	}
}

func TestConfigValidate_CBZConvertDisabledDoesNotRequireTrashPath(t *testing.T) {
	cfg := NewConfig()
	cfg.ApplyDefaults()
	cfg.Server.CBZConvert.Enabled = false
	cfg.Server.TrashPath = ""

	if err := cfg.Validate(); err != nil {
		t.Errorf("Config.Validate() should not require trash_path when cbz_convert is disabled, got: %v", err)
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
