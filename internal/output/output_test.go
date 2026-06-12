package output_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/output"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestPrintableProcessedSources(t *testing.T) {
	got := output.GetOutputItems([]types.ProcessedSource{
		{
			Name: "OpenAI",
			Info: []types.Information{
				{Title: "First", Description: "Desc"},
				{Title: "Second"},
			},
		},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 printable items, got %+v", got)
	}
	if got[0].Index != 1 || got[0].Source != "OpenAI" || got[0].Title != "First" || got[0].Description != "Desc" {
		t.Fatalf("unexpected first item: %+v", got[0])
	}
	if got[1].Index != 2 || got[1].Title != "Second" || got[1].Description != "" {
		t.Fatalf("unexpected second item: %+v", got[1])
	}
}

func TestGetOutputItemsCarriesThemeAndPersonalPreference(t *testing.T) {
	got := output.GetOutputItems([]types.ProcessedSource{
		{
			Name:               "Paris",
			Theme:              "France / Paris Local",
			PersonalPreference: 5,
			Info: []types.Information{
				{Title: "Metro disruption", Description: "Line closed"},
			},
		},
	})

	if len(got) != 1 {
		t.Fatalf("items = %+v, want 1", got)
	}
	if got[0].Theme != "France / Paris Local" || got[0].PersonalPreference != 5 {
		t.Fatalf("missing source metadata: %+v", got[0])
	}
}

func TestWriteJSONWritesValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.json")
	value := []types.OutputItem{
		{Index: 1, Source: "OpenAI", Title: "Release", Rating: 8.5},
	}

	if err := output.WriteJSON(path, value); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var got []types.OutputItem
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Release" || got[0].Rating != 8.5 {
		t.Fatalf("unexpected JSON content: %+v", got)
	}
}

func TestWriteJSONReturnsMarshalError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.json")
	err := output.WriteJSON(path, map[string]any{"bad": func() {}})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}
