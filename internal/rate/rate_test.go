package rate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestOpenRouterRaterRate(t *testing.T) {
	item := types.OutputItem{
		Source:      "OpenAI News",
		Title:       "Major model release",
		Description: "Official announcement from OpenAI.",
	}

	var gotReq types.OpenRouterChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("HTTP-Referer"); got != referer {
			t.Fatalf("HTTP-Referer = %q", got)
		}
		if got := r.Header.Get("X-Title"); got != appTitle {
			t.Fatalf("X-Title = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"score\":4.75}"}}]}`))
	}))
	defer server.Close()

	rater := OpenRouterRater{
		apiKey: "test-key",
		model:  "openai/gpt-4o-mini",
		client: server.Client(),
		url:    server.URL,
	}
	got, err := rater.Rate(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4.75 {
		t.Fatalf("rating = %f, want 4.75", got)
	}
	if gotReq.Model != "openai/gpt-4o-mini" {
		t.Fatalf("model = %q", gotReq.Model)
	}
	if gotReq.Temperature != 0 || gotReq.MaxTokens != 20 {
		t.Fatalf("unexpected generation settings: %+v", gotReq)
	}
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages length = %d", len(gotReq.Messages))
	}
	prompt := gotReq.Messages[1].Content
	for _, want := range []string{
		"Be selective",
		"Major model release",
		"OpenAI News",
		"Official announcement from OpenAI.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestParseRating(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    float64
	}{
		{name: "json", content: `{"score":4.25}`, want: 4.25},
		{name: "fallback number", content: "rating: 42", want: 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRating(tt.content)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("rating = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestRateRejectsOutOfRangeRating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"score\":6}"}}]}`))
	}))
	defer server.Close()

	rater := OpenRouterRater{
		apiKey: "test-key",
		model:  "openai/gpt-4o-mini",
		client: server.Client(),
		url:    server.URL,
	}
	_, err := rater.Rate(context.Background(), types.OutputItem{Title: "Item"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "outside 0..5") {
		t.Fatalf("error = %q", err)
	}
}
