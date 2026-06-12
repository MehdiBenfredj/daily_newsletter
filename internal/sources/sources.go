package sources

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func Collect(path string) (types.Collection, error) {
	slog.Info("collect sources started", "path", path)
	content, err := os.ReadFile(path)
	if err != nil {
		return types.Collection{}, err
	}
	slog.Info("sources file read", "path", path, "bytes", len(content))

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return types.Collection{}, err
	}

	themes, ok := root["themes"].([]any)
	if !ok {
		return types.Collection{}, fmt.Errorf("expected 'themes' to be a list in %s", path)
	}

	collected := types.Collection{
		SourcePath: filepath.Clean(path),
		ThemeCount: len(themes),
	}

	for _, themeValue := range themes {
		themeEntry, ok := themeValue.(map[string]any)
		if !ok {
			slog.Warn("skipping malformed theme entry", "path", path)
			continue
		}
		theme, ok := themeEntry["theme"].(string)
		if !ok {
			slog.Warn("skipping theme entry with missing theme name", "path", path)
			continue
		}
		sourceValues, ok := themeEntry["sources"].([]any)
		if !ok {
			slog.Warn("skipping theme with malformed sources", "path", path, "theme", theme)
			continue
		}
		slog.Info("collecting theme sources", "path", path, "theme", theme, "sources", len(sourceValues))
		for _, sourceValue := range sourceValues {
			sourceMap, ok := sourceValue.(map[string]any)
			if !ok {
				slog.Warn("skipping malformed source entry", "path", path, "theme", theme)
				continue
			}
			source, err := decodeSourceConfig(sourceMap)
			if err != nil {
				return types.Collection{}, err
			}
			if source.Type == "" {
				source.Type = "rss"
			}
			collected.Sources = append(collected.Sources, types.Source{
				Theme:              theme,
				Name:               source.Name,
				URL:                source.URL,
				Type:               source.Type,
				PersonalPreference: source.PersonalPreference,
				Config:             source,
			})
			slog.Info("source collected", "path", path, "theme", theme, "source_name", source.Name, "url", source.URL, "type", source.Type)
		}
	}
	collected.SourceCount = len(collected.Sources)
	slog.Info("collect sources completed", "path", collected.SourcePath, "themes", collected.ThemeCount, "sources", collected.SourceCount)
	return collected, nil
}

func decodeSourceConfig(value map[string]any) (types.SourceConfig, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return types.SourceConfig{}, err
	}
	var source types.SourceConfig
	if err := json.Unmarshal(raw, &source); err != nil {
		return types.SourceConfig{}, err
	}
	source.Raw = value
	return source, nil
}
