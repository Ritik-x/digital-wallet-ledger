package observability

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a structured JSON logger.
func NewLogger() *slog.Logger {
	level := slog.LevelInfo

	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler)
}
