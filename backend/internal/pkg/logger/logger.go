// Package logger cấu hình slog dùng chung cho api và worker.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New trả về logger JSON (production) hoặc text có màu-friendly (development).
func New(level, env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var h slog.Handler
	if env == "production" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}

// SetDefault gắn logger vào slog.Default để package khác dùng trực tiếp.
func SetDefault(l *slog.Logger) { slog.SetDefault(l) }

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
