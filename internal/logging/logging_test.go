package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestConfigureCreatesTimestampedLogFileInConfiguredDirectory(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(original)
	})

	logDir := t.TempDir()
	closeLog, err := Configure(logDir)
	if err != nil {
		t.Fatal(err)
	}

	slog.Info("test log message", "component", "unit-test")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}

	matches, err := filepath.Glob(filepath.Join(logDir, "*_daily_newsletter.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("log files = %v, want exactly one", matches)
	}
	name := filepath.Base(matches[0])
	if !regexp.MustCompile(`^\d{2}-\d{2}-\d{2}_\d{2}:\d{2}:\d{2}_daily_newsletter\.log$`).MatchString(name) {
		t.Fatalf("log filename = %q, want YY-MM-DD_HH:mm:ss_daily_newsletter.log", name)
	}

	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	logs := string(content)
	for _, want := range []string{
		"level=INFO",
		"source=",
		"msg=\"test log message\"",
		"component=unit-test",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log file missing %q: %s", want, logs)
		}
	}
}

func TestConfigureWithoutFileLogsToDefaultHandler(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(original)
	})

	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))

	closeLog, err := Configure("")
	if err != nil {
		t.Fatal(err)
	}
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
}
