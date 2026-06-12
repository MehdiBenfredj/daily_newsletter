package sources_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/sources"
)

func TestCollectFlattensThemesAndDefaultsType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.json")
	err := os.WriteFile(path, []byte(`{
		"themes": [
			{"theme":"AI","sources":[
				{"name":"OpenAI","url":"https://example.com/rss","personal_preference":5},
				{"name":"Site","url":"https://example.com","type":"website","max_items":7}
			]},
			{"theme":"Bad","sources":["ignored"]},
			"ignored"
		]
	}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	got, err := sources.Collect(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.SourcePath != path || got.ThemeCount != 3 || got.SourceCount != 2 {
		t.Fatalf("unexpected collection metadata: %+v", got)
	}
	if got.Sources[0].Theme != "AI" || got.Sources[0].Type != "rss" {
		t.Fatalf("missing theme/default type: %+v", got.Sources[0])
	}
	if got.Sources[1].Config.MaxItems != 7 || got.Sources[1].Type != "website" {
		t.Fatalf("missing config fields: %+v", got.Sources[1])
	}
	if got.Sources[0].PersonalPreference != 5 || got.Sources[0].Config.Raw["personal_preference"] == nil {
		t.Fatalf("missing personal preference/raw config: %+v", got.Sources[0])
	}
}

func TestCollectRejectsNonListThemes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.json")
	if err := os.WriteFile(path, []byte(`{"themes":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := sources.Collect(path); err == nil {
		t.Fatal("expected non-list themes to fail")
	}
}

func TestCollectRejectsInvalidJSONAndSourceConfigTypes(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sources.json")
		if err := os.WriteFile(path, []byte(`{"themes":[`), 0o644); err != nil {
			t.Fatal(err)
		}

		if _, err := sources.Collect(path); err == nil {
			t.Fatal("expected invalid JSON error")
		}
	})

	t.Run("invalid source field type", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sources.json")
		if err := os.WriteFile(path, []byte(`{
			"themes": [
				{"theme":"AI","sources":[
					{"name":"OpenAI","url":"https://example.com/rss","max_items":"many"}
				]}
			]
		}`), 0o644); err != nil {
			t.Fatal(err)
		}

		if _, err := sources.Collect(path); err == nil {
			t.Fatal("expected invalid source config error")
		}
	})
}

func TestCollectIgnoresMalformedThemeEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(path, []byte(`{
		"themes": [
			{"sources":[{"name":"No Theme","url":"https://example.com"}]},
			{"theme":"No Sources"},
			{"theme":"Valid","sources":[{"name":"One","url":"https://example.com/one"}]}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := sources.Collect(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThemeCount != 3 || got.SourceCount != 1 || got.Sources[0].Theme != "Valid" {
		t.Fatalf("unexpected collection: %+v", got)
	}
}
