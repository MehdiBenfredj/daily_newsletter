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
				{"name":"OpenAI","url":"https://example.com/rss","tier":5},
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
