package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestRateOutputItemsRunsInParallelAndPreservesIndexes(t *testing.T) {
	setRatingCoefficientEnv(t)
	discardLogs(t)

	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 3 {
			close(release)
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
			t.Errorf("rating requests did not run in parallel")
			return
		}

		var request types.OpenRouterChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"personal_relevance\":8,\"impact\":7,\"source_trust\":9,\"novelty\":6,\"actionability\":5,\"depth_insight\":6,\"signal_to_noise\":7}"}}]}`))
	}))
	defer server.Close()

	items := []types.OutputItem{
		{Index: 1, Source: "Source A", Title: "First"},
		{Index: 2, Source: "Source B", Title: "Second"},
		{Index: 3, Source: "Source C", Title: "Third"},
	}
	rater := types.OpenRouterRater{
		ApiKey: "test-key",
		Model:  "test-model",
		Client: server.Client(),
		Url:    server.URL,
	}

	rated := rateOutputItems(context.Background(), items, rater)
	if got := requests.Load(); got != int32(len(items)) {
		t.Fatalf("requests = %d, want %d", got, len(items))
	}
	if len(rated) != len(items) {
		t.Fatalf("rated items = %d, want %d", len(rated), len(items))
	}
	for i, item := range rated {
		if item.Index != i+1 {
			t.Fatalf("item %d index = %d, want %d", i, item.Index, i+1)
		}
		if math.Abs(item.Rating-6.04) > 0.000000001 {
			t.Fatalf("item %d rating = %f, want 6.04", i, item.Rating)
		}
	}
}

func TestRateOutputItemsSkipsAndLogsFailures(t *testing.T) {
	setRatingCoefficientEnv(t)
	logs := captureLogs(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request types.OpenRouterChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		content := `{\"personal_relevance\":8,\"impact\":7,\"source_trust\":9,\"novelty\":6,\"actionability\":5,\"depth_insight\":6,\"signal_to_noise\":7}`
		if strings.Contains(request.Messages[1].Content, "Bad item") {
			content = `{\"personal_personal_relevance\":6,\"impact\":7,\"source_trust\":10,\"novelty\":5,\"actionability\":3,\"depth_insight\":4,\"signal_to_noise\":8}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + content + `"}}]}`))
	}))
	defer server.Close()

	items := []types.OutputItem{
		{Index: 1, Source: "Source A", Title: "Good item"},
		{Index: 2, Source: "Source B", Title: "Bad item"},
	}
	rater := types.OpenRouterRater{
		ApiKey: "test-key",
		Model:  "test-model",
		Client: server.Client(),
		Url:    server.URL,
	}

	rated := rateOutputItems(context.Background(), items, rater)
	if len(rated) != 1 {
		t.Fatalf("rated items = %d, want 1", len(rated))
	}
	if rated[0].Title != "Good item" {
		t.Fatalf("rated item = %q, want Good item", rated[0].Title)
	}
	for _, want := range []string{
		"level=ERROR",
		"item rating failed; skipping item",
		"Bad item",
		`rating missing \"personal_relevance\"`,
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("logs missing %q: %s", want, logs.String())
		}
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

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	original := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(original)
	})
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	return &output
}

func setRatingCoefficientEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PERSONAL_RELEVANCE_COEF", "0.20")
	t.Setenv("IMPACT_COEF", "0.14")
	t.Setenv("SOURCE_TRUST_COEF", "0.13")
	t.Setenv("NOVELTY_COEF", "0.12")
	t.Setenv("ACTIONABILITY_COEF", "0.11")
	t.Setenv("DEPTH_INSIGHT_COEF", "0.10")
	t.Setenv("SIGNAL_TO_NOISE_COEF", "0.06")
	t.Setenv("PERSONAL_PREFERENCE_COEF", "0.14")
}
