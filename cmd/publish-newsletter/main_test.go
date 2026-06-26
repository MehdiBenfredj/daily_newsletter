package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/cache"
)

func TestRatingCacheFromEnvReturnsNilInterfaceWhenUnavailable(t *testing.T) {
	discardLogs(t)
	t.Setenv(cache.RedisURLEnv, "http://localhost:6379")

	ratingCache := ratingCacheFromEnv(context.Background())
	if ratingCache != nil {
		t.Fatalf("rating cache = %#v, want nil", ratingCache)
	}
}

func discardLogs(t *testing.T) {
	t.Helper()
	original := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(original)
	})
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}
