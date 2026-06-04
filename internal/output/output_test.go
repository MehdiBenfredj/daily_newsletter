package output_test

import (
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/newsletter"
	"github.com/MehdiBenfredj/daily_newsletter/internal/output"
)

func TestPrintableProcessedSources(t *testing.T) {
	got := output.Printable([]newsletter.ProcessedSource{
		{
			Name: "OpenAI",
			Info: []newsletter.Information{
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
