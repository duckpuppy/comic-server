package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetConfigDir returns the XDG-compliant configuration directory for comic-server
// It follows the XDG Base Directory specification:
// 1. $XDG_CONFIG_HOME/comic-server if XDG_CONFIG_HOME is set
// 2. ~/.config/comic-server otherwise
func GetConfigDir() (string, error) {
	// Check $XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "comic-server"), nil
	}

	// Fallback to ~/.config/comic-server
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(home, ".config", "comic-server"), nil
}

// GetDefaultConfigPath returns the default path to the configuration file
// It checks for existing config files in this order:
// 1. config.toml (if it exists)
// 2. config.yaml (default)
func GetDefaultConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	// Check for existing .toml file
	tomlPath := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath, nil
	}

	// Default to .yaml
	return filepath.Join(dir, "config.yaml"), nil
}

// EnsureConfigDir creates the configuration directory if it doesn't exist
// Returns the path to the config directory
func EnsureConfigDir() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	// Create directory with proper permissions (0755)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return dir, nil
}
