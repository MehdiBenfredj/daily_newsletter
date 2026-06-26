package rate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/cache"
	"github.com/MehdiBenfredj/daily_newsletter/internal/telemetry"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultModel        = "openrouter/auto"
	openRouterURL       = "https://openrouter.ai/api/v1/chat/completions"
	referer             = "https://mehdibenfredj.github.io/daily_newsletter/"
	appTitle            = "Daily Newsletter"
	randomnessFactorEnv = "RANDOMNESS_FACTOR"
)

func NewOpenRouterRater() types.OpenRouterRater {
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = defaultModel
	}
	slog.Info("openrouter rater created", "model", model, "url", openRouterURL, "has_api_key", os.Getenv("OPENROUTER_API_KEY") != "")
	return types.OpenRouterRater{
		ApiKey: os.Getenv("OPENROUTER_API_KEY"),
		Model:  model,
		Client: &http.Client{Timeout: 45 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		Url:    openRouterURL,
	}
}

func Score(r types.Rating) (float64, error) {
	slog.Info("scoring rating", "personal_relevance", r.PersonalRelevance, "impact", r.Impact, "source_trust", r.SourceTrust, "novelty", r.Novelty, "actionability", r.Actionability, "depth_insight", r.DepthInsight, "signal_to_noise", r.SignalToNoise, "personal_preference", r.PersonalPreference)
	coefficients, err := LoadRatingCoefficients()
	if err != nil {
		return 0, err
	}

	score := (r.PersonalRelevance*coefficients.PersonalRelevance +
		r.Impact*coefficients.Impact +
		r.SourceTrust*coefficients.SourceTrust +
		r.Novelty*coefficients.Novelty +
		r.Actionability*coefficients.Actionability +
		r.DepthInsight*coefficients.DepthInsight +
		r.SignalToNoise*coefficients.SignalToNoise +
		r.PersonalPreference*coefficients.PersonalPreference)

	score, err = applyRandomness(score)
	if err != nil {
		return 0, err
	}
	slog.Info("rating scored", "score", score)
	return score, nil
}

func applyRandomness(score float64) (float64, error) {
	factor, err := loadRandomnessFactor()
	if err != nil {
		return 0, err
	}
	if factor == 0 {
		return score, nil
	}

	multiplier := 1 - factor + rand.Float64()*(2*factor)
	randomized := score * multiplier
	slog.Info("rating randomness applied", "randomness_factor", factor, "multiplier", multiplier, "base_score", score, "score", randomized)
	return randomized, nil
}

func loadRandomnessFactor() (float64, error) {
	raw := strings.TrimSpace(os.Getenv(randomnessFactorEnv))
	if raw == "" {
		return 0, nil
	}

	hasPercent := strings.HasSuffix(raw, "%")
	raw = strings.TrimSpace(strings.TrimSuffix(raw, "%"))
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a percentage like 15%%", randomnessFactorEnv)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to 0", randomnessFactorEnv)
	}
	if hasPercent || value > 1 {
		value = value / 100
	}
	if value > 1 {
		return 0, fmt.Errorf("%s must be less than or equal to 100%%", randomnessFactorEnv)
	}
	return value, nil
}

func LoadRatingCoefficients() (types.RatingCoefficients, error) {
	slog.Info("loading rating coefficients")
	personalRelevanceCoef, err := strconv.ParseFloat(os.Getenv("PERSONAL_RELEVANCE_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("PERSONAL_RELEVANCE_COEF is required")
	}
	impactCoef, err := strconv.ParseFloat(os.Getenv("IMPACT_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("IMPACT_COEF is required")
	}
	sourceTrustCoef, err := strconv.ParseFloat(os.Getenv("SOURCE_TRUST_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("SOURCE_TRUST_COEF is required")
	}
	noveltyCoef, err := strconv.ParseFloat(os.Getenv("NOVELTY_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("NOVELTY_COEF is required")
	}
	actionabilityCoef, err := strconv.ParseFloat(os.Getenv("ACTIONABILITY_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("ACTIONABILITY_COEF is required")
	}
	depthInsightCoef, err := strconv.ParseFloat(os.Getenv("DEPTH_INSIGHT_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("DEPTH_INSIGHT_COEF is required")
	}
	signalToNoiseCoef, err := strconv.ParseFloat(os.Getenv("SIGNAL_TO_NOISE_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("SIGNAL_TO_NOISE_COEF is required")
	}
	personalPreferenceCoef, err := strconv.ParseFloat(os.Getenv("PERSONAL_PREFERENCE_COEF"), 64)
	if err != nil {
		return types.RatingCoefficients{}, fmt.Errorf("PERSONAL_PREFERENCE_COEF is required")
	}

	coefficients := types.RatingCoefficients{
		PersonalRelevance:  personalRelevanceCoef,
		Impact:             impactCoef,
		SourceTrust:        sourceTrustCoef,
		Novelty:            noveltyCoef,
		Actionability:      actionabilityCoef,
		DepthInsight:       depthInsightCoef,
		SignalToNoise:      signalToNoiseCoef,
		PersonalPreference: personalPreferenceCoef,
	}
	total := coefficients.PersonalRelevance +
		coefficients.Impact +
		coefficients.SourceTrust +
		coefficients.Novelty +
		coefficients.Actionability +
		coefficients.DepthInsight +
		coefficients.SignalToNoise +
		coefficients.PersonalPreference
	if total < 0.999999999 || total > 1.000000001 {
		return types.RatingCoefficients{}, fmt.Errorf("rating coefficients must sum to 1.0, got %f", total)
	}

	slog.Info("rating coefficients loaded", "personal_relevance", coefficients.PersonalRelevance, "impact", coefficients.Impact, "source_trust", coefficients.SourceTrust, "novelty", coefficients.Novelty, "actionability", coefficients.Actionability, "depth_insight", coefficients.DepthInsight, "signal_to_noise", coefficients.SignalToNoise, "personal_preference", coefficients.PersonalPreference, "total", total)
	return coefficients, nil
}

func RateInformationItems(ctx context.Context, outputItems []types.Information, rater types.OpenRouterRater, ratingCache cache.RatingCache) []types.Information {
	ctx, span := telemetry.Tracer().Start(ctx, "items.rate_batch", traceAttrs("items", len(outputItems))...)
	defer span.End()
	results := make(chan ratingResult, len(outputItems))
	var wg sync.WaitGroup
	slog.InfoContext(ctx, "rating items in parallel", "items", len(outputItems))
	for i := range outputItems {
		// Capture the current index so this goroutine writes back to the right item.
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			item := outputItems[i]
			itemCtx, itemSpan := telemetry.Tracer().Start(ctx, "item.rate", traceAttrs("item.index", item.Index, "source.name", item.Source, "item.title", item.Title)...)
			defer itemSpan.End()
			if ratingCache != nil {
				rating, err := ratingCache.Get(itemCtx, item.URL)
				if err == nil {
					itemSpan.SetAttributes(attribute.Bool("rating.cache_hit", true))
					slog.InfoContext(itemCtx, "item rating loaded from cache", "index", item.Index, "source_name", item.Source, "title", item.Title, "url", item.URL, "rating", rating)
					results <- ratingResult{index: i, rating: rating}
					return
				}
				itemSpan.SetAttributes(attribute.Bool("rating.cache_hit", false))
				if !errors.Is(err, cache.ErrCacheMiss) {
					slog.WarnContext(itemCtx, "item rating cache lookup failed; rating item", "index", item.Index, "source_name", item.Source, "title", item.Title, "url", item.URL, "error", err)
				}
			}
			slog.InfoContext(itemCtx, "rating item", "index", item.Index, "source_name", item.Source, "title", item.Title)
			rating, err := Rate(itemCtx, item, rater)
			if err != nil {
				telemetry.RecordSpanError(itemSpan, err)
				results <- ratingResult{index: i, err: fmt.Errorf("rate item %d (%q): %w", item.Index, item.Title, err)}
				return
			}
			if ratingCache != nil {
				if err := ratingCache.Set(itemCtx, item.URL, rating); err != nil {
					slog.WarnContext(itemCtx, "item rating cache write failed", "index", item.Index, "source_name", item.Source, "title", item.Title, "url", item.URL, "rating", rating, "error", err)
				} else {
					slog.InfoContext(itemCtx, "item rating cached", "index", item.Index, "source_name", item.Source, "title", item.Title, "url", item.URL, "rating", rating)
				}
			}
			results <- ratingResult{index: i, rating: rating}
		}()
	}
	wg.Wait()
	close(results)

	failed := make([]bool, len(outputItems))
	for result := range results {
		item := outputItems[result.index]
		if result.err != nil {
			failed[result.index] = true
			telemetry.ItemsSkipped.Add(ctx, 1)
			slog.ErrorContext(ctx, "item rating failed; skipping item", "index", item.Index, "source_name", item.Source, "title", item.Title, "error", result.err)
			continue
		}
		outputItems[result.index].Rating = result.rating
		telemetry.ItemsRated.Add(ctx, 1)
		slog.InfoContext(ctx, "item rated", "index", item.Index, "source_name", item.Source, "rating", result.rating)
	}

	rated := make([]types.Information, 0, len(outputItems))
	for i, item := range outputItems {
		if !failed[i] {
			rated = append(rated, item)
		}
	}
	span.SetAttributes(attribute.Int("rated_items", len(rated)), attribute.Int("skipped_items", len(outputItems)-len(rated)))
	slog.InfoContext(ctx, "rating items finished", "items", len(outputItems), "rated_items", len(rated), "skipped_items", len(outputItems)-len(rated))
	return rated
}

func Rate(ctx context.Context, item types.Information, r types.OpenRouterRater) (float64, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "openrouter.rate_item", traceAttrs("item.index", item.Index, "source.name", item.Source, "item.title", item.Title, "item.theme", item.Theme)...)
	defer span.End()
	slog.InfoContext(ctx, "rate item started", "index", item.Index, "source_name", item.Source, "title", item.Title, "theme", item.Theme)
	if r.ApiKey == "" {
		err := fmt.Errorf("OPENROUTER_API_KEY is required")
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 45 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	}
	if r.Url == "" {
		r.Url = openRouterURL
	}

	reqBody := types.OpenRouterChatRequest{
		Model: r.Model,
		Messages: []types.OpenRouterMessage{
			{Role: "system", Content: "You are a strict newsletter curator. Return only valid JSON."},
			{Role: "user", Content: promptFor(item)},
		},
		Temperature: 0,
		MaxTokens:   120,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	span.SetAttributes(attribute.String("openrouter.model", r.Model), attribute.Int("request.bytes", len(body)))
	slog.InfoContext(ctx, "openrouter request prepared", "index", item.Index, "model", r.Model, "url", r.Url, "bytes", len(body))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Url, bytes.NewReader(body))
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+r.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", referer)
	req.Header.Set("X-Title", appTitle)

	start := time.Now()
	telemetry.OpenRouterRequests.Add(ctx, 1)
	resp, err := r.Client.Do(req)
	telemetry.OpenRouterLatencyMS.Record(ctx, telemetry.DurationMillis(start))
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	defer resp.Body.Close()
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	slog.InfoContext(ctx, "openrouter response received", "index", item.Index, "status", resp.Status)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("openrouter returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
		telemetry.RecordSpanError(span, err)
		slog.ErrorContext(ctx, "openrouter returned non-success status", "index", item.Index, "status", resp.Status, "body", strings.TrimSpace(string(respBody)))
		return 0, err
	}

	var chatResp types.OpenRouterChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	if len(chatResp.Choices) == 0 {
		err := fmt.Errorf("openrouter returned no choices")
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	slog.InfoContext(ctx, "openrouter choices parsed", "index", item.Index, "choices", len(chatResp.Choices))

	rating, err := parseRating(chatResp.Choices[0].Message.Content)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	if err := validateRating(rating); err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	rating.PersonalPreference = float64(item.PersonalPreference)
	score, err := Score(rating)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	span.SetAttributes(attribute.Float64("rating.score", score))
	slog.InfoContext(ctx, "rate item completed", "index", item.Index, "source_name", item.Source, "rating", score)
	return score, nil
}

func traceAttrs(values ...any) []trace.SpanStartOption {
	attrs := make([]attribute.KeyValue, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			continue
		}
		switch value := values[i+1].(type) {
		case string:
			attrs = append(attrs, attribute.String(key, value))
		case int:
			attrs = append(attrs, attribute.Int(key, value))
		case bool:
			attrs = append(attrs, attribute.Bool(key, value))
		case float64:
			attrs = append(attrs, attribute.Float64(key, value))
		}
	}
	return []trace.SpanStartOption{trace.WithAttributes(attrs...)}
}

func promptFor(item types.Information) string {
	return fmt.Sprintf(prompt, item.Source, item.Title, item.Description, item.Theme)
}

func parseRating(content string) (types.Rating, error) {
	slog.Info("parsing rating response", "bytes", len(content))
	var rating types.Rating
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &rating); err != nil {
		return types.Rating{}, fmt.Errorf("invalid rating JSON: %w", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &fields); err != nil {
		return types.Rating{}, fmt.Errorf("invalid rating JSON: %w", err)
	}

	for _, field := range []string{
		"personal_relevance",
		"impact",
		"source_trust",
		"novelty",
		"actionability",
		"depth_insight",
		"signal_to_noise",
	} {
		if _, ok := fields[field]; !ok {
			return types.Rating{}, fmt.Errorf("rating missing %q from response %q", field, content)
		}
	}

	slog.Info("rating response parsed", "personal_relevance", rating.PersonalRelevance, "impact", rating.Impact, "source_trust", rating.SourceTrust, "novelty", rating.Novelty, "actionability", rating.Actionability, "depth_insight", rating.DepthInsight, "signal_to_noise", rating.SignalToNoise)
	return rating, nil
}

func validateRating(rating types.Rating) error {
	slog.Info("validating rating")
	for name, value := range map[string]float64{
		"personal_relevance": rating.PersonalRelevance,
		"impact":             rating.Impact,
		"source_trust":       rating.SourceTrust,
		"novelty":            rating.Novelty,
		"actionability":      rating.Actionability,
		"depth_insight":      rating.DepthInsight,
		"signal_to_noise":    rating.SignalToNoise,
	} {
		if value < 0 || value > 10 {
			return fmt.Errorf("rating %s=%f outside 0..10", name, value)
		}
	}
	slog.Info("rating validated")
	return nil
}

type ratingResult struct {
	index  int
	rating float64
	err    error
}
