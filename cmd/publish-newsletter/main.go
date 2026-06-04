package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/fetch"
	"github.com/MehdiBenfredj/daily_newsletter/internal/newsletter"
	"github.com/MehdiBenfredj/daily_newsletter/internal/output"
	"github.com/MehdiBenfredj/daily_newsletter/internal/parse"
	"github.com/MehdiBenfredj/daily_newsletter/internal/sources"
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

	defaultSources := filepath.Join(repo, "site", "sources.json")
	sourcesPath := flag.String("sources", defaultSources, "path to sources.json")
	flag.Parse()

	collection, err := sources.Collect(*sourcesPath)
	if err != nil {
		return err
	}

	processed := processSources(collection)
	for _, source := range processed {
		fmt.Printf("%s: %d information items\n", source.Name, len(source.Info))
	}

	return output.WriteJSON(filepath.Join(repo, "processed_sources.json"), output.Printable(processed))
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

func processSources(collection newsletter.Collection) []newsletter.ProcessedSource {
	processed := make([]newsletter.ProcessedSource, 0, len(collection.Sources))
	now := time.Now().UTC()
	for _, source := range collection.Sources {
		item := newsletter.ProcessedSource{
			Theme:  source.Theme,
			Name:   source.Name,
			URL:    source.URL,
			Type:   source.Type,
			Config: source.Config,
			Tier:   source.Tier,
		}
		result, err := processSource(source)
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			processed = append(processed, item)
			continue
		}
		item.OK = true
		item.Processed = &result
		info, err := parse.ProcessedSource(item, now)
		if err != nil {
			item.ParseError = err.Error()
		} else {
			item.Info = info
		}
		processed = append(processed, item)
	}
	return processed
}

func processSource(source newsletter.Source) (newsletter.Processed, error) {
	raw, err := fetch.Source(source)
	if err != nil {
		return newsletter.Processed{}, err
	}
	sourceType := strings.ToLower(source.Type)
	if sourceType == "" {
		sourceType = "rss"
	}
	text := string(raw)
	switch sourceType {
	case "rss", "feed", "atom", "xml":
		return newsletter.Processed{ContentType: "rss", Bytes: len(raw), Data: text}, nil
	case "website", "html", "web":
		return newsletter.Processed{ContentType: "website", Bytes: len(raw), Data: text}, nil
	case "api":
		var data any
		if err := json.Unmarshal(raw, &data); err != nil {
			data = text
		}
		return newsletter.Processed{ContentType: "api", Bytes: len(raw), Data: data}, nil
	default:
		return newsletter.Processed{}, fmt.Errorf("unsupported source type %q for %s", sourceType, source.Name)
	}
}
