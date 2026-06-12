package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const LogDirEnv = "LOG_DIR"

func ConfigureFromEnv() (func() error, error) {
	return Configure(os.Getenv(LogDirEnv))
}

func Configure(logDir string) (func() error, error) {
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
	slog.SetDefault(slog.New(handler))

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
