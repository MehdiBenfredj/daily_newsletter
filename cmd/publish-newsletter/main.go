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
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/env"
	"github.com/MehdiBenfredj/daily_newsletter/internal/fetch"
	"github.com/MehdiBenfredj/daily_newsletter/internal/logging"
	"github.com/MehdiBenfredj/daily_newsletter/internal/output"
	"github.com/MehdiBenfredj/daily_newsletter/internal/parse"
	"github.com/MehdiBenfredj/daily_newsletter/internal/rate"
	"github.com/MehdiBenfredj/daily_newsletter/internal/sources"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
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
	closeLog, err := logging.ConfigureFromEnv()
	if err != nil {
		return err
	}
	defer func() {
		if err := closeLog(); err != nil {
			fmt.Fprintf(os.Stderr, "close log file: %v\n", err)
		}
	}()
	slog.Info("starting newsletter publishing", "repo", repo)

	defaultSources := filepath.Join(repo, "site", "sources.json")
	sourcesPath := flag.String("sources", defaultSources, "path to sources.json")
	flag.Parse()
	slog.Info("loading sources", "path", *sourcesPath)

	collection, err := sources.Collect(*sourcesPath)
	if err != nil {
		return err
	}
	slog.Info("sources loaded", "path", collection.SourcePath, "themes", collection.ThemeCount, "sources", collection.SourceCount)

	processed, errored := processSources(collection)
	for _, source := range processed {
		slog.Info("source processed", "source_name", source.Name, "items", len(source.Info), "theme", source.Theme, "type", source.Type)
	}
	slog.Info("source processing completed", "processed", len(processed), "errored", len(errored))
	outputItems := output.GetOutputItems(processed)
	slog.Info("output items prepared", "items", len(outputItems))

	rater := rate.NewOpenRouterRater()
	for i := range outputItems {
		slog.Info("rating item", "index", outputItems[i].Index, "source_name", outputItems[i].Source, "title", outputItems[i].Title)
		rating, err := rate.Rate(context.Background(), outputItems[i], rater)
		if err != nil {
			return fmt.Errorf("rate item %d (%q): %w", outputItems[i].Index, outputItems[i].Title, err)
		}
		outputItems[i].Rating = rating
		slog.Info("item rated", "index", outputItems[i].Index, "source_name", outputItems[i].Source, "rating", rating)
	}

	sort.Slice(outputItems, func(i, j int) bool {
		return outputItems[i].Rating > outputItems[j].Rating
	})
	slog.Info("items sorted", "items", len(outputItems))

	outputPath := filepath.Join(repo, "processed_sources.json")
	if err := output.WriteJSON(outputPath, outputItems); err != nil {
		return err
	}
	slog.Info("newsletter publishing completed", "output", outputPath, "items", len(outputItems))
	return nil
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

func processSources(collection types.Collection) ([]types.ProcessedSource, []types.ProcessedSource) {
	processed := make([]types.ProcessedSource, 0, len(collection.Sources))
	errored := make([]types.ProcessedSource, 0, len(collection.Sources))
	now := time.Now().UTC()
	for _, source := range collection.Sources {
		slog.Info("processing source", "source_name", source.Name, "url", source.URL, "theme", source.Theme, "type", source.Type)
		item := types.ProcessedSource{
			Theme:              source.Theme,
			Name:               source.Name,
			URL:                source.URL,
			Type:               source.Type,
			Config:             source.Config,
			PersonalPreference: source.PersonalPreference,
		}
		result, err := processSource(source)
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			errored = append(errored, item)
			slog.Error("source processing failed", "source_name", source.Name, "url", source.URL, "error", err)
			continue
		}
		item.OK = true
		item.Processed = &result
		slog.Info("source fetched", "source_name", source.Name, "content_type", result.ContentType, "bytes", result.Bytes)
		info, err := parse.ProcessedSource(item, now)
		if err != nil {
			item.ParseError = err.Error()
			slog.Error("source parsing failed", "source_name", source.Name, "error", err)
		} else {
			item.Info = info
			slog.Info("source parsed", "source_name", source.Name, "items", len(info))
		}
		processed = append(processed, item)
	}
	return processed, errored
}

func processSource(source types.Source) (types.Processed, error) {
	slog.Info("fetching source", "source_name", source.Name, "url", source.URL, "type", source.Type)
	raw, err := fetch.Source(source)
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
