package rate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

const (
	defaultModel  = "openrouter/auto"
	openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
	referer       = "https://mehdibenfredj.github.io/daily_newsletter/"
	appTitle      = "Daily Newsletter"
)

func NewOpenRouterRater() types.OpenRouterRater {
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = defaultModel
	}
	return types.OpenRouterRater{
		ApiKey: os.Getenv("OPENROUTER_API_KEY"),
		Model:  model,
		Client: &http.Client{Timeout: 45 * time.Second},
		Url:    openRouterURL,
	}
}

func Score(r types.Rating) (float64, error) {
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
	return score, nil
}

func LoadRatingCoefficients() (types.RatingCoefficients, error) {
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

	return coefficients, nil
}

func Rate(ctx context.Context, item types.OutputItem, r types.OpenRouterRater) (float64, error) {
	if r.ApiKey == "" {
		return 0, fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 45 * time.Second}
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
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+r.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", referer)
	req.Header.Set("X-Title", appTitle)

	resp, err := r.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("openrouter returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var chatResp types.OpenRouterChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return 0, err
	}
	if len(chatResp.Choices) == 0 {
		return 0, fmt.Errorf("openrouter returned no choices")
	}

	rating, err := parseRating(chatResp.Choices[0].Message.Content)
	if err != nil {
		return 0, err
	}
	if err := validateRating(rating); err != nil {
		return 0, err
	}
	rating.PersonalPreference = float64(item.PersonalPreference)
	return Score(rating)
}

func promptFor(item types.OutputItem) string {
	return fmt.Sprintf(prompt, item.Source, item.Title, item.Description, item.Theme)
}

func parseRating(content string) (types.Rating, error) {
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

	return rating, nil
}

func validateRating(rating types.Rating) error {
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
	return nil
}
