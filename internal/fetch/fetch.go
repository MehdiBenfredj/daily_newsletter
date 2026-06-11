package fetch

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

const (
	userAgent = "daily-newsletter/1.0 (+https://github.com/)"
	timeout   = 45 * time.Second
	retries   = 3
)

func Source(source types.Source) ([]byte, error) {
	if source.URL == "" {
		return nil, fmt.Errorf("%s is missing a URL", source.Name)
	}

	headers, err := headersFor(source)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: timeout}
	if source.Config.InsecureSSL {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	url := source.URL
	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		body, status, err := fetchOnce(client, url, headers)
		if err == nil && status < 400 {
			return body, nil
		}
		if status == http.StatusForbidden && source.Config.FallbackURL != "" && url != source.Config.FallbackURL {
			url = source.Config.FallbackURL
			attempt = -1
			continue
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("http %d fetching %s", status, url)
		}
		if !isRetriable(status, err) {
			return nil, lastErr
		}
		if attempt < retries-1 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	return nil, lastErr
}

func headersFor(source types.Source) (http.Header, error) {
	headers := http.Header{}
	headers.Set("User-Agent", userAgent)
	headers.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.7")
	headers.Set("Accept-Encoding", "identity")
	headers.Set("Connection", "close")

	if source.Config.Auth == "apiKey" {
		key := os.Getenv("PRIM_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("%s requires PRIM_API_KEY", source.Name)
		}
		headers.Set("apikey", key)
	}
	return headers, nil
}

func fetchOnce(client *http.Client, url string, headers http.Header) ([]byte, int, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header = headers.Clone()

	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()

	body, readErr := io.ReadAll(response.Body)
	return body, response.StatusCode, readErr
}

func isRetriable(status int, err error) bool {
	if err != nil {
		return true
	}
	return status == http.StatusTooManyRequests || status >= 500 || errors.Is(err, io.ErrUnexpectedEOF)
}
