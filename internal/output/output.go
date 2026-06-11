package output

import (
	"encoding/json"
	"os"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func GetOutputItems(sources []types.ProcessedSource) []types.OutputItem {
	var items []types.OutputItem
	index := 1
	for _, source := range sources {
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
			if source.Tier != 0 {
				item.Tier = source.Tier
			}
			if source.Theme != "" {
				item.Theme = source.Theme
			}
			items = append(items, item)
			index++
		}
	}
	return items
}

func WriteJSON(path string, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
