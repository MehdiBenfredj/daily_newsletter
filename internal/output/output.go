package output

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func GetOutputItems(sources []types.ProcessedSource) []types.OutputItem {
	slog.Info("building output items", "sources", len(sources))
	var items []types.OutputItem
	index := 1
	for _, source := range sources {
		slog.Info("building output items for source", "source_name", source.Name, "information_items", len(source.Info))
		for _, information := range source.Info {
			item := types.OutputItem{
				Index:  index,
				Source: source.Name,
			}
			if information.Title != "" {
				item.Title = information.Title
			}
			if information.Description != "" {
				item.Description = information.Description
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
