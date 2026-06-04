package output

import (
	"encoding/json"
	"os"

	"github.com/MehdiBenfredj/daily_newsletter/internal/newsletter"
)

func Printable(sources []newsletter.ProcessedSource) []newsletter.PrintableItem {
	var items []newsletter.PrintableItem
	index := 1
	for _, source := range sources {
		for _, information := range source.Info {
			item := newsletter.PrintableItem{
				Index:  index,
				Source: source.Name,
			}
			if information.Title != "" {
				item.Title = information.Title
			}
			if information.Description != "" {
				item.Description = information.Description
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
