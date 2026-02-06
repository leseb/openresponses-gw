// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"io"
	"log/slog"
	"os"
)

// Config for logger
type Config struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "json" or "text"
	Output io.Writer
}

// Logger wraps slog.Logger
type Logger struct {
	*slog.Logger
}

// New creates a new logger
func New(cfg Config) *Logger {
	// Parse level
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Set output
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	// Create handler
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}
