package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"
)

const LogDirEnv = "LOG_DIR"

func ConfigureFromEnv(loggerProvider *sdklog.LoggerProvider) (func() error, error) {
	return Configure(os.Getenv(LogDirEnv), loggerProvider)
}

func Configure(logDir string, loggerProvider *sdklog.LoggerProvider) (func() error, error) {
	writers := []io.Writer{os.Stdout}
	var file *os.File
	var logFile string

	if logDir != "" {
		var err error
		logDir, err = expandHomeDir(logDir)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, err
		}
		logFile = filepath.Join(logDir, time.Now().Format("06-01-02_15:04:05")+"_daily_newsletter.log")
		opened, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		file = opened
		writers = append(writers, file)
	}

	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelInfo,
	})
	var defaultHandler slog.Handler = traceContextHandler{next: handler}
	if loggerProvider != nil {
		otelHandler := otelslog.NewHandler("github.com/MehdiBenfredj/daily_newsletter", otelslog.WithLoggerProvider(loggerProvider), otelslog.WithSource(true))
		defaultHandler = multiHandler{handlers: []slog.Handler{defaultHandler, otelHandler}}
	}
	slog.SetDefault(slog.New(defaultHandler))

	if logDir == "" {
		slog.Warn("log directory env var is not set; logging to stdout only", "env_var", LogDirEnv)
	} else {
		slog.Info("logging configured", "log_dir", logDir, "log_file", logFile)
	}

	return func() error {
		if file == nil {
			return nil
		}
		return file.Close()
	}, nil
}

type multiHandler struct {
	handlers []slog.Handler
}

func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var err error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			err = joinErrors(err, handler.Handle(ctx, record.Clone()))
		}
	}
	return err
}

func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return multiHandler{handlers: handlers}
}

func (h multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return multiHandler{handlers: handlers}
}

type traceContextHandler struct {
	next slog.Handler
}

func (h traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h traceContextHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, record)
}

func (h traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceContextHandler{next: h.next.WithAttrs(attrs)}
}

func (h traceContextHandler) WithGroup(name string) slog.Handler {
	return traceContextHandler{next: h.next.WithGroup(name)}
}

func joinErrors(left, right error) error {
	if left != nil {
		return left
	}
	return right
}

func expandHomeDir(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}
