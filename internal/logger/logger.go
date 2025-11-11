package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Config holds logging configuration
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, text
}

// Setup initializes the global logger with the given configuration
func Setup(cfg Config) error {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(level)

	// Set output format
	var output io.Writer = os.Stdout
	if cfg.Format == "text" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	// Configure global logger
	log.Logger = zerolog.New(output).
		With().
		Timestamp().
		Caller().
		Logger()

	return nil
}

// With creates a new logger with additional fields
func With() zerolog.Context {
	return log.With()
}

// Info returns an info level event
func Info() *zerolog.Event {
	return log.Info()
}

// Debug returns a debug level event
func Debug() *zerolog.Event {
	return log.Debug()
}

// Warn returns a warn level event
func Warn() *zerolog.Event {
	return log.Warn()
}

// Error returns an error level event
func Error() *zerolog.Event {
	return log.Error()
}

// Fatal returns a fatal level event
func Fatal() *zerolog.Event {
	return log.Fatal()
}
