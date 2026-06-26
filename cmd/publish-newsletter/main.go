package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/cache"
	"github.com/MehdiBenfredj/daily_newsletter/internal/env"
	"github.com/MehdiBenfredj/daily_newsletter/internal/logging"
	"github.com/MehdiBenfredj/daily_newsletter/internal/output"
	"github.com/MehdiBenfredj/daily_newsletter/internal/process"
	"github.com/MehdiBenfredj/daily_newsletter/internal/rate"
	"github.com/MehdiBenfredj/daily_newsletter/internal/sources"
	"github.com/MehdiBenfredj/daily_newsletter/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
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
	err = env.Load(filepath.Join(repo, ".env"))
	if err != nil {
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

	ctx, span := telemetry.Tracer().Start(context.Background(), "publish_newsletter.run", telemetry.TraceAttrs("repo", repo)...)
	defer span.End()
	slog.InfoContext(ctx, "starting newsletter publishing", "repo", repo)

	defaultSources := filepath.Join(repo, "site", "sources.json")
	sourcesPath := flag.String("sources", defaultSources, "path to sources.json")
	flag.Parse()
	slog.InfoContext(ctx, "loading sources", "path", *sourcesPath)

	loadCtx, loadSpan := telemetry.Tracer().Start(ctx, "sources.collect", telemetry.TraceAttrs("sources.path", *sourcesPath)...)
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

	processed, errored := process.ProcessSources(ctx, collection)
	for _, source := range processed {
		slog.InfoContext(ctx, "source processed", "source_name", source.Name, "items", len(source.Info), "theme", source.Theme, "type", source.Type)
	}
	slog.InfoContext(ctx, "source processing completed", "processed", len(processed), "errored", len(errored))
	informationItems := output.EnrichInformationItems(processed)
	slog.InfoContext(ctx, "output items prepared", "items", len(informationItems))

	rater := rate.NewOpenRouterRater()
	ratingCache := ratingCacheFromEnv(ctx)
	informationItems = rate.RateInformationItems(ctx, informationItems, rater, ratingCache)
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

func ratingCacheFromEnv(ctx context.Context) cache.RatingCache {
	var ratingCache cache.RatingCache
	redisRatingCache, cacheEnabled, err := cache.NewRedisRatingCacheFromEnv()
	if err != nil {
		slog.WarnContext(ctx, "rating cache unavailable; continuing without cache", "error", err)
	}
	if cacheEnabled {
		ratingCache = redisRatingCache
	}
	return ratingCache
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
