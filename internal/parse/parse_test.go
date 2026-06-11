package parse_test

import (
	"testing"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
	"github.com/MehdiBenfredj/daily_newsletter/internal/parse"
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
	rss := types.Processed{Data: `<rss><channel>
		<item><title>One &amp; Two</title><link>https://example.com/1</link><pubDate>Tue, 02 Jun 2026 10:00:00 GMT</pubDate><description><![CDATA[Hello&nbsp;world]]></description></item>
		<item><title>Old</title><link>https://example.com/old</link><pubDate>Sun, 31 May 2026 10:00:00 GMT</pubDate></item>
	</channel></rss>`}
	got, err := parse.ProcessedSource(types.ProcessedSource{Type: "rss", Processed: &rss}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "One & Two" || got[0].Description != "Hello world" {
		t.Fatalf("unexpected rss items: %+v", got)
	}

	atom := types.Processed{Data: `<feed xmlns="http://www.w3.org/2005/Atom">
		<entry><title>Atom</title><link href="https://example.com/a"/><updated>2026-06-02T11:00:00Z</updated><summary>Summary</summary></entry>
	</feed>`}
	got, err = parse.ProcessedSource(types.ProcessedSource{Type: "atom", Processed: &atom}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected atom items: %+v", got)
	}
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
