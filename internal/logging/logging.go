// Package logging configures the application's structured logger.
//
// The logger is built on the standard library log/slog so that no extra
// dependency is introduced. The level and handler format are driven by
// configuration; production typically uses JSON, development uses readable
// text.
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// New builds a *slog.Logger writing to stdout.
//   - level: one of debug, info, warn, error (case-insensitive; "" = info)
//   - format: "json" or "text" (case-insensitive; "" = text)
func New(level, format string) (*slog.Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch strings.ToLower(format) {
	case "", "text":
		h = slog.NewTextHandler(os.Stdout, opts)
	case "json":
		h = slog.NewJSONHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("invalid log format %q: want json or text", format)
	}
	return slog.New(h), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q: want debug|info|warn|error", s)
	}
}
