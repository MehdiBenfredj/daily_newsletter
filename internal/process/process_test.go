package process_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/process"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestProcessSourceMapsSourceTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(`<rss><channel><item><title>Item</title></item></channel></rss>`))
		case "/html":
			_, _ = w.Write([]byte(`<html><body><a href="/news">News</a></body></html>`))
		case "/api":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/text-api":
			_, _ = w.Write([]byte(`not json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cases := []struct {
		name        string
		sourceType  string
		path        string
		contentType string
	}{
		{name: "default rss", path: "/feed", contentType: "rss"},
		{name: "explicit rss", sourceType: "feed", path: "/feed", contentType: "rss"},
		{name: "website", sourceType: "html", path: "/html", contentType: "website"},
		{name: "api json", sourceType: "api", path: "/api", contentType: "api"},
		{name: "api text fallback", sourceType: "api", path: "/text-api", contentType: "api"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := process.ProcessSource(context.Background(), types.Source{
				Name: "Example",
				Type: tt.sourceType,
				URL:  server.URL + tt.path,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.ContentType != tt.contentType {
				t.Fatalf("content type = %q, want %q", got.ContentType, tt.contentType)
			}
			if got.Bytes == 0 {
				t.Fatal("bytes = 0, want fetched content length")
			}
			if tt.sourceType == "api" && tt.path == "/api" {
				data, ok := got.Data.(map[string]any)
				if !ok || data["status"] != "ok" {
					t.Fatalf("api data = %#v, want decoded JSON", got.Data)
				}
			}
			if tt.path == "/text-api" && got.Data != "not json" {
				t.Fatalf("api fallback data = %#v, want raw text", got.Data)
			}
		})
	}
}

func TestProcessSourceRejectsUnsupportedType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	_, err := process.ProcessSource(context.Background(), types.Source{
		Name: "Example",
		Type: "pdf",
		URL:  server.URL,
	})
	if err == nil || !strings.Contains(err.Error(), `unsupported source type "pdf" for Example`) {
		t.Fatalf("error = %v", err)
	}
}

func TestProcessSourcesProcessesAndSeparatesErroredSources(t *testing.T) {
	published := time.Now().UTC().Add(-time.Hour).Format(time.RFC1123Z)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<rss><channel>
			<item>
				<title>Fresh item</title>
				<link>https://example.com/fresh</link>
				<pubDate>%s</pubDate>
				<description>Useful item</description>
			</item>
		</channel></rss>`, published)
	}))
	defer server.Close()

	processed, errored := process.ProcessSources(context.Background(), types.Collection{
		Sources: []types.Source{
			{
				Theme:              "Tech",
				Name:               "Feed",
				URL:                server.URL,
				Type:               "rss",
				PersonalPreference: 2,
			},
			{
				Theme: "Broken",
				Name:  "Missing URL",
				Type:  "rss",
			},
		},
	})

	if len(processed) != 1 {
		t.Fatalf("processed sources = %d, want 1", len(processed))
	}
	if len(errored) != 1 {
		t.Fatalf("errored sources = %d, want 1", len(errored))
	}
	if !processed[0].OK || processed[0].Processed == nil {
		t.Fatalf("processed source = %+v, want successful fetched source", processed[0])
	}
	if processed[0].PersonalPreference != 2 {
		t.Fatalf("personal preference = %d, want 2", processed[0].PersonalPreference)
	}
	if len(processed[0].Info) != 1 || processed[0].Info[0].Title != "Fresh item" {
		t.Fatalf("parsed info = %+v, want fresh item", processed[0].Info)
	}
	if errored[0].OK || !strings.Contains(errored[0].Error, "Missing URL is missing a URL") {
		t.Fatalf("errored source = %+v, want missing URL error", errored[0])
	}
}
