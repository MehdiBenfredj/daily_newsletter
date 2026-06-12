package rate

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/env"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestOpenRouterRaterRate(t *testing.T) {
	item := types.Information{
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
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"personal_relevance\":8,\"impact\":7,\"source_trust\":9,\"novelty\":6,\"actionability\":5,\"depth_insight\":6,\"signal_to_noise\":7}"}}]}`))
	}))
	defer server.Close()

	rater := newTestOpenRouterRater(t, server)
	got, err := Rate(context.Background(), item, rater)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-6.04) > 0.000000001 {
		t.Fatalf("rating = %f, want 6.04", got)
	}
	if gotReq.Model != "openai/gpt-4o-mini" {
		t.Fatalf("model = %q", gotReq.Model)
	}
	if gotReq.Temperature != 0 || gotReq.MaxTokens != 120 {
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
		want    types.Rating
	}{
		{
			name:    "json",
			content: `{"personal_relevance":8,"impact":7,"source_trust":9,"novelty":6,"actionability":5,"depth_insight":6,"signal_to_noise":7}`,
			want: types.Rating{
				PersonalRelevance: 8,
				Impact:            7,
				SourceTrust:       9,
				Novelty:           6,
				Actionability:     5,
				DepthInsight:      6,
				SignalToNoise:     7,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRating(tt.content)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("rating = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseRatingRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "invalid json",
			content: `rating: 7`,
			want:    "invalid rating JSON",
		},
		{
			name:    "missing field",
			content: `{"personal_relevance":8,"impact":7,"source_trust":9,"novelty":6,"actionability":5,"depth_insight":6}`,
			want:    `rating missing "signal_to_noise"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRating(tt.content)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRateRejectsOutOfRangeRating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"personal_relevance\":11,\"impact\":7,\"source_trust\":9,\"novelty\":6,\"actionability\":5,\"depth_insight\":6,\"signal_to_noise\":7}"}}]}`))
	}))
	defer server.Close()

	rater := newTestOpenRouterRater(t, server)
	_, err := Rate(context.Background(), types.Information{Title: "Item"}, rater)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "outside 0..10") {
		t.Fatalf("error = %q", err)
	}
}

func TestLoadRatingCoefficients(t *testing.T) {
	setTestCoefficients(t)

	got, err := LoadRatingCoefficients()
	if err != nil {
		t.Fatal(err)
	}
	if got.PersonalRelevance != 0.20 || got.SourceTrust != 0.13 || got.PersonalPreference != 0.14 {
		t.Fatalf("unexpected coefficients: %+v", got)
	}
}

func TestLoadRatingCoefficientsRejectsMissingAndInvalidSum(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		setTestCoefficients(t)
		t.Setenv("SOURCE_TRUST_COEF", "")

		_, err := LoadRatingCoefficients()
		if err == nil || !strings.Contains(err.Error(), "SOURCE_TRUST_COEF is required") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid sum", func(t *testing.T) {
		setTestCoefficients(t)
		t.Setenv("SOURCE_TRUST_COEF", "0.14")

		_, err := LoadRatingCoefficients()
		if err == nil || !strings.Contains(err.Error(), "must sum to 1.0") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestScoreAppliesRandomnessFactor(t *testing.T) {
	setTestCoefficients(t)
	t.Setenv(randomnessFactorEnv, "15%")

	rating := types.Rating{
		PersonalRelevance:  8,
		Impact:             7,
		SourceTrust:        9,
		Novelty:            6,
		Actionability:      5,
		DepthInsight:       6,
		SignalToNoise:      7,
		PersonalPreference: 4,
	}
	base := 6.60

	for range 100 {
		got, err := Score(rating)
		if err != nil {
			t.Fatal(err)
		}
		if got < base*0.85 || got > base*1.15 {
			t.Fatalf("randomized score = %f, want within [%f, %f]", got, base*0.85, base*1.15)
		}
	}
}

func TestScoreWithoutRandomnessIsDeterministic(t *testing.T) {
	setTestCoefficients(t)

	rating := types.Rating{
		PersonalRelevance:  8,
		Impact:             7,
		SourceTrust:        9,
		Novelty:            6,
		Actionability:      5,
		DepthInsight:       6,
		SignalToNoise:      7,
		PersonalPreference: 4,
	}

	for range 10 {
		got, err := Score(rating)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-6.60) > 0.000000001 {
			t.Fatalf("score = %f, want deterministic 6.60", got)
		}
	}
}

func TestScoreReturnsRandomnessConfigError(t *testing.T) {
	setTestCoefficients(t)
	t.Setenv(randomnessFactorEnv, "nope")

	_, err := Score(types.Rating{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), randomnessFactorEnv) {
		t.Fatalf("error = %q, want %s", err, randomnessFactorEnv)
	}
}

func TestLoadRandomnessFactor(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  float64
	}{
		{name: "missing", value: "", want: 0},
		{name: "zero percent", value: "0%", want: 0},
		{name: "percent", value: "15%", want: 0.15},
		{name: "percent with spaces", value: " 15 % ", want: 0.15},
		{name: "whole number", value: "15", want: 0.15},
		{name: "fraction", value: "0.15", want: 0.15},
		{name: "one percent as whole", value: "1%", want: 0.01},
		{name: "one as fraction max", value: "1", want: 1},
		{name: "hundred percent", value: "100%", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(randomnessFactorEnv, tt.value)

			got, err := loadRandomnessFactor()
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-tt.want) > 0.000000001 {
				t.Fatalf("randomness factor = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestLoadRandomnessFactorRejectsInvalidValues(t *testing.T) {
	tests := []string{"nope", "-1%", "101%"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv(randomnessFactorEnv, value)

			_, err := loadRandomnessFactor()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRateHandlesOpenRouterErrors(t *testing.T) {
	t.Run("non success status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}))
		defer server.Close()

		rater := newTestOpenRouterRater(t, server)
		_, err := Rate(context.Background(), types.Information{Title: "Item"}, rater)
		if err == nil || !strings.Contains(err.Error(), "openrouter returned 502 Bad Gateway") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("no choices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"choices":[]}`))
		}))
		defer server.Close()

		rater := newTestOpenRouterRater(t, server)
		_, err := Rate(context.Background(), types.Information{Title: "Item"}, rater)
		if err == nil || !strings.Contains(err.Error(), "openrouter returned no choices") {
			t.Fatalf("error = %v", err)
		}
	})
}

func newTestOpenRouterRater(t *testing.T, server *httptest.Server) types.OpenRouterRater {
	t.Helper()
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_MODEL", "")
	if err := env.Load(filepath.Join("..", "..", ".env.test")); err != nil {
		t.Fatal(err)
	}

	rater := NewOpenRouterRater()
	rater.Client = server.Client()
	rater.Url = server.URL
	return rater
}

func setTestCoefficients(t *testing.T) {
	t.Helper()
	t.Setenv("PERSONAL_RELEVANCE_COEF", "0.20")
	t.Setenv("IMPACT_COEF", "0.14")
	t.Setenv("SOURCE_TRUST_COEF", "0.13")
	t.Setenv("NOVELTY_COEF", "0.12")
	t.Setenv("ACTIONABILITY_COEF", "0.11")
	t.Setenv("DEPTH_INSIGHT_COEF", "0.10")
	t.Setenv("SIGNAL_TO_NOISE_COEF", "0.06")
	t.Setenv("PERSONAL_PREFERENCE_COEF", "0.14")
	t.Setenv(randomnessFactorEnv, "")
}
