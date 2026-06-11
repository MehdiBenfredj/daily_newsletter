package fetch_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MehdiBenfredj/daily_newsletter/internal/fetch"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

func TestFetchSendsHeadersAndAPIKey(t *testing.T) {
	t.Setenv("PRIM_API_KEY", "secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "daily-newsletter/") {
			t.Fatalf("unexpected user agent %q", got)
		}
		if got := r.Header.Get("Accept-Encoding"); got != "identity" {
			t.Fatalf("unexpected accept encoding %q", got)
		}
		if got := r.Header.Get("apikey"); got != "secret" {
			t.Fatalf("unexpected api key %q", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	body, err := fetch.Source(types.Source{URL: server.URL, Config: types.SourceConfig{Auth: "apiKey"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", string(body))
	}
}

func TestFetchFallsBackOnForbiddenAndRetriesTransientFailures(t *testing.T) {
	attempts := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("fallback"))
	}))
	defer fallback.Close()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer primary.Close()

	body, err := fetch.Source(types.Source{
		URL: primary.URL,
		Config: types.SourceConfig{
			FallbackURL: fallback.URL,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fallback" || attempts != 2 {
		t.Fatalf("expected retried fallback, body=%q attempts=%d", string(body), attempts)
	}
}

func TestFetchRequiresURLAndAPIKey(t *testing.T) {
	if _, err := fetch.Source(types.Source{}); err == nil {
		t.Fatal("expected missing URL error")
	}
	if _, err := fetch.Source(types.Source{URL: "http://example.test", Config: types.SourceConfig{Auth: "apiKey"}}); err == nil {
		t.Fatal("expected missing API key error")
	}
}
