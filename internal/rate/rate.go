package rate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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

type OpenRouterRater struct {
	apiKey string
	model  string
	client *http.Client
	url    string
}

func NewOpenRouterRater() OpenRouterRater {
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = defaultModel
	}
	return OpenRouterRater{
		apiKey: os.Getenv("OPENROUTER_API_KEY"),
		model:  model,
		client: &http.Client{Timeout: 45 * time.Second},
		url:    openRouterURL,
	}
}

func (r OpenRouterRater) Rate(ctx context.Context, item types.OutputItem) (float64, error) {
	if r.apiKey == "" {
		return 0, fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	if r.client == nil {
		r.client = &http.Client{Timeout: 45 * time.Second}
	}
	if r.url == "" {
		r.url = openRouterURL
	}

	reqBody := types.OpenRouterChatRequest{
		Model: r.model,
		Messages: []types.OpenRouterMessage{
			{Role: "system", Content: "You are a strict newsletter curator. Return only valid JSON."},
			{Role: "user", Content: promptFor(item)},
		},
		Temperature: 0,
		MaxTokens:   20,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", referer)
	req.Header.Set("X-Title", appTitle)

	resp, err := r.client.Do(req)
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
	if rating < 0 || rating > 5 {
		return 0, fmt.Errorf("rating %f outside 0..5", rating)
	}
	return rating, nil
}

func promptFor(item types.OutputItem) string {
	return fmt.Sprintf(`You are the editor of my personal daily newsletter. Your job is to decide whether an information item deserves my attention today.

Goal:
Replace passive scrolling on X with a concise, high-signal, personally relevant newsletter. Be selective. Prefer fewer, better items.

Reader profile:

* I am a software engineer based in Paris from Algeria
* I care about AI/agentic systems, software engineering, architecture, football, FC Barcelona, politics/geopolitics, general news, Algeria, and France/Paris local information.
* I value practical insight, trustworthy information, strategic awareness, and things that may affect my work, plans, worldview, or daily life.
* I dislike clickbait, shallow hype, duplicate news, low-quality rumors, and generic content.

Input:
You will receive one information item with:


* source name
* title
* description
* source tier from 1 to 5
* theme/category

Evaluate the item using the rubric below.

Scoring dimensions:

1. Personal relevance, 0-5
   How relevant is this to my interests, location, work, or daily life?

2. Source trust, 0-5
   Use the source tier as a starting point, but adjust based on the content.

* 5 = official source, primary source, highly reputable wire, expert source
* 4 = reputable specialist source
* 3 = useful but potentially biased, fan-oriented, or mixed quality
* 2 = low confidence, rumor-heavy, unclear sourcing
* 1 = unreliable or mostly clickbait

3. Novelty, 0-5
   Is this actually new or does it add something meaningful beyond what is already known?
   Penalize reposts, generic announcements, repeated rumors, and minor updates.

4. Impact, 0-5
   How much could this matter?
   Consider technical impact, political impact, financial/economic impact, local-life impact, football importance, or strategic significance.

5. Depth / insight, 0-5
   Does the item teach me something, explain a mechanism, reveal a trend, contain data, provide analysis, or expose useful trade-offs?

6. Actionability, 0-5
   Can I do something with this information?
   Examples:

* change how I build software
* investigate a tool/model/paper
* prepare for a local disruption
* understand a political shift
* watch a football match/event
* save an article for deeper reading
* share with someone relevant

7. Signal-to-noise, 0-5
   Is the item concise, factual, specific, and non-clickbait?
   Penalize hype, vague claims, weak evidence, SEO content, speculation, and emotional bait.

Theme-specific guidance:

* AI / Agentic:
  Reward concrete capability changes, new research, agent frameworks, benchmarks, API/product changes, safety implications, cost/performance changes, and practical developer relevance.
  Penalize vague “AI will change everything” content.

* Software Engineering / Architecture:
  Reward reusable lessons, architecture decisions, incident reports, scalability lessons, performance data, trade-offs, patterns, and strong technical explanations.
  Penalize product marketing with little engineering substance.

* Football / FC Barcelona:
  Reward official confirmations, tactical analysis, match importance, injuries, transfers from reliable sources, and data-driven analysis.
  Penalize unconfirmed rumors and repetitive transfer speculation.

* Politics / Geopolitics:
  Reward primary sources, major policy changes, elections, war/peace developments, EU/French/Algerian relevance, and serious analysis.
  Penalize partisan outrage, weakly sourced claims, and opinion masquerading as fact.

* General News:
  Reward major events with broad consequences, France/Europe relevance, and stories that change the context of the day.
  Penalize routine crime, celebrity noise, and generic breaking-news churn.

* France / Paris Local:
  Reward practical local impact: transport disruption, strikes, safety alerts, weather alerts, administrative changes, public services, major local events, housing, mobility, or Paris life.
  Actionability matters more here than global importance.

Compute:
Use this weighted formula:

final_score =
0.22 * personal_relevance +
0.16 * impact +
0.15 * source_trust +
0.14 * novelty +
0.13 * actionability +
0.12 * depth_insight +
0.08 * signal_to_noise

Output:
Return ONLY this JSON object, with no text before or after it, no markdown fences, no explanation:
{"score": <value>}

Where <value> is the final_score rounded to two decimal places

Rules:

* Be strict. Most items should not be “Must Read.”
* Do not reward an item only because the source is famous.
* Do not reward an item only because the topic is popular.
* Prefer durable insight over breaking noise.
* Prefer primary sources and well-sourced reporting.
* Penalize duplicate or near-duplicate items.
* Penalize speculation unless the source is highly reliable and the potential impact is high.
* For local Paris information, practical usefulness can outweigh global importance.
* For AI and software engineering, technical substance matters more than hype.
* If the content is insufficient to judge, lower confidence and avoid high scores.

Item:
Source: %s
Title: %s
Description: %s
Tier: %d
Theme: %s`, item.Source, item.Title, item.Description, item.Tier, item.Theme)
}

func parseRating(content string) (float64, error) {
	var payload struct {
		Rating float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &payload); err == nil {
		return payload.Rating, nil
	}

	match := regexp.MustCompile(`\d+`).FindString(content)
	if match == "" {
		return 0, fmt.Errorf("rating missing from response %q", content)
	}
	rating, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rating format: %w", err)
	}
	return rating, nil
}
