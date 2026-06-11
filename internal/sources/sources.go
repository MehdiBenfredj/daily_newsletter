package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func Collect(path string) (types.Collection, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return types.Collection{}, err
	}

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
			continue
		}
		theme, ok := themeEntry["theme"].(string)
		if !ok {
			continue
		}
		sourceValues, ok := themeEntry["sources"].([]any)
		if !ok {
			continue
		}
		for _, sourceValue := range sourceValues {
			sourceMap, ok := sourceValue.(map[string]any)
			if !ok {
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
				Theme:  theme,
				Name:   source.Name,
				URL:    source.URL,
				Type:   source.Type,
				Tier:   source.Tier,
				Config: source,
			})
		}
	}
	collected.SourceCount = len(collected.Sources)
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
