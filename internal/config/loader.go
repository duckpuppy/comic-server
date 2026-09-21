package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// Load reads a configuration file from the specified path. Format is
// auto-detected based on file extension (.yaml or .toml).
//
// A missing or empty config file is not an error - it just means there's
// nothing to unmarshal on top of NewConfig()'s defaults. Either way,
// ApplyDefaults/ApplyEnvironment/Validate always run before returning
// (comic-server-n5ms: they used to be skipped entirely on a missing/empty
// file, which meant every COMIC_SERVER_* environment variable was
// silently ignored on a fresh install with no config.yaml on disk yet -
// exactly the case a first-run Docker container configured purely by env
// vars hits).
func Load(path string) (*Config, error) {
	config := NewConfig()

	data, err := os.ReadFile(path)
	switch {
	case err != nil && os.IsNotExist(err):
		// No config file yet - config stays at NewConfig()'s defaults,
		// still goes through the common tail below.
	case err != nil:
		return nil, fmt.Errorf("failed to read config file: %w", err)
	case len(data) == 0:
		// Empty file - same as "no file", nothing to unmarshal.
	default:
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse YAML config: %w", err)
			}
		case ".toml":
			if err := toml.Unmarshal(data, config); err != nil {
				return nil, fmt.Errorf("failed to parse TOML config: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported config format: %s (use .yaml or .toml)", ext)
		}
	}

	// Initialize map if nil (happens with empty YAML/TOML, or no file at all)
	if config.Devices == nil {
		config.Devices = make(map[string]*DeviceConfig)
	}

	// Apply defaults for missing values
	config.ApplyDefaults()

	// Apply environment variable overrides
	if err := config.ApplyEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to apply environment variables: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// Save writes configuration to the specified path
// Format is determined by file extension (.yaml or .toml)
func Save(config *Config, path string) error {
	// Ensure config directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Detect format from extension
	ext := strings.ToLower(filepath.Ext(path))

	var data []byte
	var err error

	switch ext {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config to YAML: %w", err)
		}
	case ".toml":
		data, err = toml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config to TOML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config format: %s (use .yaml or .toml)", ext)
	}

	// Write file with proper permissions (0644)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadDefault loads the configuration from the default XDG-compliant path
func LoadDefault() (*Config, error) {
	path, err := GetDefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return Load(path)
}

// SaveDefault saves the configuration to the default XDG-compliant path
func SaveDefault(config *Config) error {
	path, err := GetDefaultConfigPath()
	if err != nil {
		return err
	}
	return Save(config, path)
}
