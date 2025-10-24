package cmd

import (
	"fmt"
	"os"

	"github.com/duckpuppy/comic-server/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "comic-server",
	Short: "A headless ComicRack wireless sync server",
	Long: `comic-server is a standalone headless server that implements the ComicRack
wireless synchronization protocol for syncing comic books to Android/iOS devices.

It provides device discovery, library management, and wireless sync capabilities
without requiring the full ComicRack desktop application.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	configFile string
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $XDG_CONFIG_HOME/comic-server/config.yaml)")
}

// GetConfigPath returns the config file path, using the flag value or default
func GetConfigPath() (string, error) {
	if configFile != "" {
		return configFile, nil
	}
	return config.GetDefaultConfigPath()
}
