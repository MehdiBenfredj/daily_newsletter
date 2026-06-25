package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/env"
	"github.com/MehdiBenfredj/daily_newsletter/internal/fetch"
	"github.com/MehdiBenfredj/daily_newsletter/internal/logging"
	"github.com/MehdiBenfredj/daily_newsletter/internal/output"
	"github.com/MehdiBenfredj/daily_newsletter/internal/parse"
	"github.com/MehdiBenfredj/daily_newsletter/internal/rate"
	"github.com/MehdiBenfredj/daily_newsletter/internal/sources"
	"github.com/MehdiBenfredj/daily_newsletter/internal/telemetry"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repo, err := repoRoot()
	if err != nil {
		return err
	}
	if err := env.Load(filepath.Join(repo, ".env")); err != nil {
		return err
	}
	telemetryProviders, err := telemetry.Init(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure telemetry: %v\n", err)
	}
	var loggerProvider *sdklog.LoggerProvider
	if telemetryProviders != nil {
		loggerProvider = telemetryProviders.LoggerProvider
	}
	closeLog, err := logging.ConfigureFromEnv(loggerProvider)
	if err != nil {
		return err
	}
	defer func() {
		if err := closeLog(); err != nil {
			fmt.Fprintf(os.Stderr, "close log file: %v\n", err)
		}
	}()
	defer func() {
		if telemetryProviders == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetryProviders.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown telemetry: %v\n", err)
		}
	}()

	ctx, span := telemetry.Tracer().Start(context.Background(), "publish_newsletter.run", traceAttrs("repo", repo)...)
	defer span.End()
	slog.InfoContext(ctx, "starting newsletter publishing", "repo", repo)

	defaultSources := filepath.Join(repo, "site", "sources.json")
	sourcesPath := flag.String("sources", defaultSources, "path to sources.json")
	flag.Parse()
	slog.InfoContext(ctx, "loading sources", "path", *sourcesPath)

	loadCtx, loadSpan := telemetry.Tracer().Start(ctx, "sources.collect", traceAttrs("sources.path", *sourcesPath)...)
	collection, err := sources.Collect(*sourcesPath)
	if err != nil {
		telemetry.RecordSpanError(loadSpan, err)
		loadSpan.End()
		telemetry.RecordSpanError(span, err)
		return err
	}
	loadSpan.SetAttributes(attribute.Int("themes", collection.ThemeCount), attribute.Int("sources", collection.SourceCount))
	loadSpan.End()
	slog.InfoContext(loadCtx, "sources loaded", "path", collection.SourcePath, "themes", collection.ThemeCount, "sources", collection.SourceCount)

	processed, errored := processSources(ctx, collection)
	for _, source := range processed {
		slog.InfoContext(ctx, "source processed", "source_name", source.Name, "items", len(source.Info), "theme", source.Theme, "type", source.Type)
	}
	slog.InfoContext(ctx, "source processing completed", "processed", len(processed), "errored", len(errored))
	informationItems := output.EnrichInformationItems(processed)
	slog.InfoContext(ctx, "output items prepared", "items", len(informationItems))

	rater := rate.NewOpenRouterRater()
	informationItems = rateInformationItems(ctx, informationItems, rater)
	slog.InfoContext(ctx, "rating completed", "rated_items", len(informationItems))

	sort.Slice(informationItems, func(i, j int) bool {
		return informationItems[i].Rating > informationItems[j].Rating
	})
	slog.InfoContext(ctx, "items sorted", "items", len(informationItems))

	outputPath := filepath.Join(repo, "processed_informations.json")
	if err := output.WriteJSON(outputPath, informationItems); err != nil {
		telemetry.RecordSpanError(span, err)
		return err
	}
	slog.InfoContext(ctx, "newsletter publishing completed", "output", outputPath, "items", len(informationItems))
	return nil
}

type ratingResult struct {
	index  int
	rating float64
	err    error
}

func rateInformationItems(ctx context.Context, outputItems []types.Information, rater types.OpenRouterRater) []types.Information {
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
			slog.InfoContext(itemCtx, "rating item", "index", item.Index, "source_name", item.Source, "title", item.Title)
			rating, err := rate.Rate(itemCtx, item, rater)
			if err != nil {
				telemetry.RecordSpanError(itemSpan, err)
				results <- ratingResult{index: i, err: fmt.Errorf("rate item %d (%q): %w", item.Index, item.Title, err)}
				return
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

func repoRoot() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := workingDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "site", "sources.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root from %s", workingDir)
		}
	}
}

func processSources(ctx context.Context, collection types.Collection) ([]types.ProcessedSource, []types.ProcessedSource) {
	ctx, span := telemetry.Tracer().Start(ctx, "sources.process", traceAttrs("sources", len(collection.Sources))...)
	defer span.End()
	processed := make([]types.ProcessedSource, 0, len(collection.Sources))
	errored := make([]types.ProcessedSource, 0, len(collection.Sources))
	now := time.Now().UTC()
	for _, source := range collection.Sources {
		sourceCtx, sourceSpan := telemetry.Tracer().Start(ctx, "source.process", traceAttrs("source.name", source.Name, "source.url", source.URL, "source.theme", source.Theme, "source.type", source.Type)...)
		slog.InfoContext(sourceCtx, "processing source", "source_name", source.Name, "url", source.URL, "theme", source.Theme, "type", source.Type)
		item := types.ProcessedSource{
			Theme:              source.Theme,
			Name:               source.Name,
			URL:                source.URL,
			Type:               source.Type,
			Config:             source.Config,
			PersonalPreference: source.PersonalPreference,
		}
		result, err := processSource(sourceCtx, source)
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			errored = append(errored, item)
			telemetry.RecordSpanError(sourceSpan, err)
			telemetry.SourcesFailed.Add(sourceCtx, 1)
			slog.ErrorContext(sourceCtx, "source processing failed", "source_name", source.Name, "url", source.URL, "error", err)
			sourceSpan.End()
			continue
		}
		item.OK = true
		item.Processed = &result
		sourceSpan.SetAttributes(attribute.String("content.type", result.ContentType), attribute.Int("content.bytes", result.Bytes))
		slog.InfoContext(sourceCtx, "source fetched", "source_name", source.Name, "content_type", result.ContentType, "bytes", result.Bytes)
		parseCtx, parseSpan := telemetry.Tracer().Start(sourceCtx, "source.parse", traceAttrs("source.name", source.Name, "content.type", result.ContentType)...)
		info, err := parse.ProcessedSource(item, now)
		if err != nil {
			item.ParseError = err.Error()
			telemetry.RecordSpanError(parseSpan, err)
			slog.ErrorContext(parseCtx, "source parsing failed", "source_name", source.Name, "error", err)
		} else {
			item.Info = info
			parseSpan.SetAttributes(attribute.Int("items", len(info)))
			slog.InfoContext(parseCtx, "source parsed", "source_name", source.Name, "items", len(info))
		}
		parseSpan.End()
		telemetry.SourcesProcessed.Add(sourceCtx, 1)
		sourceSpan.SetAttributes(attribute.Int("items", len(item.Info)))
		sourceSpan.End()
		processed = append(processed, item)
	}
	span.SetAttributes(attribute.Int("processed", len(processed)), attribute.Int("errored", len(errored)))
	return processed, errored
}

func processSource(ctx context.Context, source types.Source) (types.Processed, error) {
	slog.InfoContext(ctx, "fetching source", "source_name", source.Name, "url", source.URL, "type", source.Type)
	raw, err := fetch.SourceContext(ctx, source)
	if err != nil {
		return types.Processed{}, err
	}
	sourceType := strings.ToLower(source.Type)
	if sourceType == "" {
		sourceType = "rss"
	}
	text := string(raw)
	switch sourceType {
	case "rss", "feed", "atom", "xml":
		return types.Processed{ContentType: "rss", Bytes: len(raw), Data: text}, nil
	case "website", "html", "web":
		return types.Processed{ContentType: "website", Bytes: len(raw), Data: text}, nil
	case "api":
		var data any
		if err := json.Unmarshal(raw, &data); err != nil {
			data = text
		}
		return types.Processed{ContentType: "api", Bytes: len(raw), Data: data}, nil
	default:
		return types.Processed{}, fmt.Errorf("unsupported source type %q for %s", sourceType, source.Name)
	}
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
