package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestInit_TextFormat(t *testing.T) {
	// Initialize with text format
	err := Init(Config{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Verify logger is initialized
	if Logger.GetLevel() != 1 { // info level = 1 in zerolog
		t.Errorf("Expected info level (1), got %d", Logger.GetLevel())
	}
}

func TestInit_JSONFormat(t *testing.T) {
	var buf bytes.Buffer

	// Initialize with JSON format
	err := Init(Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Redirect logger to buffer for testing
	Logger = Logger.Output(&buf)

	// Log a test message
	Logger.Info().Str("test", "value").Msg("test message")

	// Parse JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v\nOutput: %s", err, buf.String())
	}

	// Verify fields
	if logEntry["level"] != "info" {
		t.Errorf("Expected level 'info', got %v", logEntry["level"])
	}
	if logEntry["message"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", logEntry["message"])
	}
	if logEntry["test"] != "value" {
		t.Errorf("Expected test field 'value', got %v", logEntry["test"])
	}
}

func TestInit_InvalidLevel(t *testing.T) {
	// Invalid level should default to info
	err := Init(Config{
		Level:  "invalid",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Should default to info level (1)
	if Logger.GetLevel() != 1 {
		t.Errorf("Expected info level (1) as default, got %d", Logger.GetLevel())
	}
}

func TestInit_DebugLevel(t *testing.T) {
	err := Init(Config{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Debug level = 0 in zerolog
	if Logger.GetLevel() != 0 {
		t.Errorf("Expected debug level (0), got %d", Logger.GetLevel())
	}
}

func TestInit_ErrorLevel(t *testing.T) {
	err := Init(Config{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Error level = 3 in zerolog
	if Logger.GetLevel() != 3 {
		t.Errorf("Expected error level (3), got %d", Logger.GetLevel())
	}
}

func TestLoggerFunctions(t *testing.T) {
	var buf bytes.Buffer

	// Initialize logger
	err := Init(Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Redirect to buffer
	Logger = Logger.Output(&buf)

	// Test each level function
	tests := []struct {
		name     string
		logFunc  func() *zerolog.Event
		expected string
	}{
		{"debug", Debug, "debug"},
		{"info", Info, "info"},
		{"warn", Warn, "warn"},
		{"error", Error, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc().Msg("test")

			// Verify log level in output
			if !strings.Contains(buf.String(), tt.expected) {
				t.Errorf("Expected log to contain level '%s', got: %s", tt.expected, buf.String())
			}
		})
	}
}
