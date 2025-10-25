package log

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var (
	// Logger is the global logger instance
	Logger zerolog.Logger
)

// Config holds logging configuration
type Config struct {
	Level  string // debug, info, warn, error
	Format string // text, json
	Output string // stdout, stderr, file path
}

func init() {
	// Configure zerolog to use pkgerrors for stack trace support
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// Set default logger (will be configured by Init)
	Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

// Init initializes the global logger with the provided configuration
func Init(cfg Config) error {
	// Parse log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Create writer based on output config
	var writer io.Writer
	switch cfg.Output {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		// Open file for logging
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		writer = file
	}

	// Configure format
	if cfg.Format == "text" {
		writer = zerolog.ConsoleWriter{
			Out:        writer,
			TimeFormat: time.RFC3339,
		}
	}

	// Create logger
	Logger = zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Logger()

	return nil
}

// Debug logs a debug message
func Debug() *zerolog.Event {
	return Logger.Debug()
}

// Info logs an info message
func Info() *zerolog.Event {
	return Logger.Info()
}

// Warn logs a warning message
func Warn() *zerolog.Event {
	return Logger.Warn()
}

// Error logs an error message
func Error() *zerolog.Event {
	return Logger.Error()
}

// Fatal logs a fatal message and exits
func Fatal() *zerolog.Event {
	return Logger.Fatal()
}

// With creates a child logger with additional context
func With() zerolog.Context {
	return Logger.With()
}
