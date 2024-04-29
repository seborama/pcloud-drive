package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const RFC3339Milli = "2006-01-02T15:04:05.999Z07:00"

func LoggerSetup() {
	replace := func(groups []string, a slog.Attr) slog.Attr {
		// change the time format.
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.String(a.Key, a.Value.Time().Format(time.TimeOnly))
		}
		return a
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{
			AddSource:   false,
			Level:       slog.LevelInfo,
			ReplaceAttr: replace,
		},
	)))
	// slog.SetDefault(slog.New(slog.NewJSONHandler(
	// 	os.Stdout,
	// 	&slog.HandlerOptions{
	// 		AddSource:   false,
	// 		Level:       slog.LevelInfo,
	// 		ReplaceAttr: replace,
	// 	},
	// )))
}

// Debugf wraps slog.Log with caller info from the stacktrace.
func Debugf(msg string, args ...any) {
	log(slog.LevelDebug, msg, args...)
}

// Infof wraps slog.Log with caller info from the stacktrace.
func Infof(msg string, args ...any) {
	log(slog.LevelInfo, msg, args...)
}

// Warnf wraps slog.Log with caller info from the stacktrace.
func Warnf(msg string, args ...any) {
	log(slog.LevelWarn, msg, args...)
}

// Errorf wraps slog.Log with caller info from the stacktrace.
func Errorf(msg string, args ...any) {
	log(slog.LevelError, msg, args...)
}

func log(level slog.Level, msg string, args ...any) {
	logger := slog.Default()
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip [Callers, Infof, log]
	fs := runtime.CallersFrames([]uintptr{pcs[0]})
	f, _ := fs.Next()
	caller := fmt.Sprintf("%s/%s:%d", filepath.Base(f.File), filepath.Base(f.Function), f.Line)

	slog.Log(context.Background(), level, msg, append([]any{slog.String("caller", caller)}, args...)...)
}
