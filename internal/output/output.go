package output

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func EnrichInformationItems(sources []types.ProcessedSource) []types.Information {
	slog.Info("building output items", "sources", len(sources))
	var items []types.Information
	index := 1
	for _, source := range sources {
		slog.Info("building output items for source", "source_name", source.Name, "information_items", len(source.Info))
		for _, information := range source.Info {
			item := information
			item.Index = index
			if source.Name != "" {
				item.Source = source.Name
			}
			if source.PersonalPreference != 0 {
				item.PersonalPreference = source.PersonalPreference
			}
			if source.Theme != "" {
				item.Theme = source.Theme
			}
			items = append(items, item)
			index++
		}
	}
	slog.Info("output items built", "items", len(items))
	return items
}

func WriteJSON(path string, value any) error {
	slog.Info("writing json output", "path", path)
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return err
	}
	slog.Info("json output written", "path", path, "bytes", len(content))
	return nil
}
