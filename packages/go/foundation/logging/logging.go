// Package logging provides the shared structured logging setup.
package logging

import (
	"io"
	"log/slog"
)

func New(output io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level}))
}
