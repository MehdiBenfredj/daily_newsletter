package parse_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/parse"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestParsePublishedDatetimeAndRecentWindow(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cases := []string{
		"Tue, 02 Jun 2026 10:00:00 GMT",
		"2026-06-02T10:00:00Z",
		"2026-06-02T10:00:00+00:00",
		"2026-06-02T10:00:00",
	}
	for _, value := range cases {
		if !parse.WasPublishedInLast24Hours(value, now) {
			t.Fatalf("expected %q to be recent", value)
		}
	}
	if parse.WasPublishedInLast24Hours("2026-05-31T10:00:00Z", now) {
		t.Fatal("expected old date to be filtered")
	}
	if parse.WasPublishedInLast24Hours("not a date", now) {
		t.Fatal("expected invalid date to be filtered")
	}
}

func TestRSSAndAtomParsing(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rss := types.Processed{Data: `<rss xmlns:media="http://search.yahoo.com/mrss/"><channel>
		<item><title>One &amp; Two</title><link>https://example.com/1</link><pubDate>Tue, 02 Jun 2026 10:00:00 GMT</pubDate><description><![CDATA[Hello&nbsp;world]]></description><media:thumbnail url="https://example.com/one.jpg"/></item>
		<item><title>Old</title><link>https://example.com/old</link><pubDate>Sun, 31 May 2026 10:00:00 GMT</pubDate></item>
	</channel></rss>`}
	got, err := parse.ProcessedSource(types.ProcessedSource{Type: "rss", Processed: &rss}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "One & Two" || got[0].Description != "Hello world" || got[0].ImageURL != "https://example.com/one.jpg" {
		t.Fatalf("unexpected rss items: %+v", got)
	}

	atom := types.Processed{Data: `<feed xmlns="http://www.w3.org/2005/Atom">
		<entry><title>Atom</title><link href="https://example.com/a"/><updated>2026-06-02T11:00:00Z</updated><summary>Summary</summary><content url="https://example.com/atom.webp" type="image/webp"/></entry>
	</feed>`}
	got, err = parse.ProcessedSource(types.ProcessedSource{Type: "atom", Processed: &atom}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://example.com/a" || got[0].ImageURL != "https://example.com/atom.webp" {
		t.Fatalf("unexpected atom items: %+v", got)
	}
}

func TestProcessedSourceRejectsMalformedInputs(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	t.Run("nil processed source", func(t *testing.T) {
		got, err := parse.ProcessedSource(types.ProcessedSource{Type: "rss"}, now)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("items = %+v, want nil", got)
		}
	})

	t.Run("invalid xml", func(t *testing.T) {
		processed := types.Processed{Data: `<rss><channel><item></channel></rss>`}
		_, err := parse.ProcessedSource(types.ProcessedSource{Type: "rss", Processed: &processed}, now)
		if err == nil {
			t.Fatal("expected invalid XML error")
		}
	})

	t.Run("invalid website regex", func(t *testing.T) {
		source := types.ProcessedSource{
			URL:       "https://example.com",
			Type:      "website",
			Config:    types.SourceConfig{IncludeURLRegex: "["},
			Processed: &types.Processed{Data: `<a href="/news">News item title</a>`},
		}
		_, err := parse.ProcessedSource(source, now)
		if err == nil {
			t.Fatal("expected regex error")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := parse.ProcessedSource(types.ProcessedSource{
			Name:      "Example",
			Type:      "pdf",
			Processed: &types.Processed{Data: "content"},
		}, now)
		if err == nil || !strings.Contains(err.Error(), `unsupported source type "pdf"`) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestWebsiteParsing(t *testing.T) {
	source := types.ProcessedSource{
		URL:  "https://example.com/root/",
		Type: "website",
		Config: types.SourceConfig{
			IncludeURLRegex: "/news/",
			ExcludeURLRegex: "skip",
			MaxItems:        2,
		},
		Processed: &types.Processed{Data: `<html><body>
			<a href="/news/one#fragment"> First useful article </a>
			<a href="/news/one"> Duplicate useful article </a>
			<a href="/news/skip"> Skipped article title </a>
			<a href="/blog/two"> Wrong section article </a>
			<a href="/news/two"> Second useful article </a>
			<a href="/news/three"> Third useful article </a>
		</body></html>`},
	}

	got, err := parse.ProcessedSource(source, time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].URL != "https://example.com/news/one" || got[1].URL != "https://example.com/news/two" {
		t.Fatalf("unexpected website items: %+v", got)
	}
}

func TestWebsiteParsingHandlesRelativeAndEmptyLinks(t *testing.T) {
	source := types.ProcessedSource{
		URL:  "https://example.com/root/page",
		Type: "website",
		Processed: &types.Processed{Data: `<html><body>
			<a href=""> Empty link title </a>
			<a href="#local"> Same page fragment </a>
			<a href="../news/one?x=1#fragment"> Relative useful article </a>
			<a href="https://other.example.com/story"> External useful article </a>
		</body></html>`},
	}

	got, err := parse.ProcessedSource(source, time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("items = %+v, want 2", got)
	}
	if got[0].URL != "https://example.com/news/one?x=1" || got[0].Title != "Relative useful article" {
		t.Fatalf("unexpected first item: %+v", got[0])
	}
	if got[1].URL != "https://other.example.com/story" {
		t.Fatalf("unexpected second item: %+v", got[1])
	}
}

func TestAPIParsing(t *testing.T) {
	source := types.ProcessedSource{
		URL:  "https://api.example.com",
		Type: "api",
		Processed: &types.Processed{Data: map[string]any{
			"lines": []any{
				map[string]any{"id": "line:1", "mode": "metro", "shortName": "1"},
			},
			"disruptions": []any{
				map[string]any{
					"title":      "Delay",
					"lastUpdate": "2026-06-02T11:00:00Z",
					"severity":   "major",
					"cause":      "works",
					"message":    "<p>Use another route&nbsp;today</p>",
					"impactedSections": []any{
						map[string]any{"lineId": "line:1"},
					},
				},
			},
		}},
	}

	got, err := parse.ProcessedSource(source, time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Delay" || got[0].Description != "metro 1 | major | works | Use another route today" {
		t.Fatalf("unexpected api items: %+v", got)
	}
}
