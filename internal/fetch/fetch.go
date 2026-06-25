package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/telemetry"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	userAgent = "daily-newsletter/1.0 (+https://github.com/)"
	timeout   = 45 * time.Second
	retries   = 3
)

func Source(source types.Source) ([]byte, error) {
	return SourceContext(context.Background(), source)
}

func SourceContext(ctx context.Context, source types.Source) ([]byte, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "source.fetch", traceAttrs("source.name", source.Name, "source.url", source.URL, "source.type", source.Type)...)
	defer span.End()

	if source.URL == "" {
		err := fmt.Errorf("%s is missing a URL", source.Name)
		telemetry.RecordSpanError(span, err)
		return nil, err
	}
	slog.InfoContext(ctx, "fetch source started", "source_name", source.Name, "url", source.URL, "type", source.Type)

	headers, err := headersFor(source)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return nil, err
	}

	client := &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	if source.Config.InsecureSSL {
		client.Transport = otelhttp.NewTransport(&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}})
	}

	url := source.URL
	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		attemptCtx, attemptSpan := telemetry.Tracer().Start(ctx, "source.fetch_attempt", traceAttrs("source.name", source.Name, "url", url, "attempt", attempt+1)...)
		start := time.Now()
		telemetry.FetchAttempts.Add(attemptCtx, 1)
		slog.InfoContext(attemptCtx, "fetch attempt started", "source_name", source.Name, "url", url, "attempt", attempt+1, "max_attempts", retries)
		body, status, err := fetchOnce(attemptCtx, client, url, headers)
		telemetry.SourceFetchLatencyMS.Record(attemptCtx, telemetry.DurationMillis(start))
		attemptSpan.SetAttributes(attribute.Int("http.response.status_code", status), attribute.Int("response.bytes", len(body)))
		if err == nil && status < 400 {
			slog.InfoContext(attemptCtx, "fetch attempt succeeded", "source_name", source.Name, "url", url, "status", status, "bytes", len(body))
			attemptSpan.End()
			span.SetAttributes(attribute.Int("response.bytes", len(body)), attribute.Int("http.response.status_code", status))
			return body, nil
		}
		if status == http.StatusForbidden && source.Config.FallbackURL != "" && url != source.Config.FallbackURL {
			slog.WarnContext(attemptCtx, "fetch forbidden; switching to fallback url", "source_name", source.Name, "url", url, "fallback_url", source.Config.FallbackURL, "status", status)
			attemptSpan.End()
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
			telemetry.FetchFailures.Add(attemptCtx, 1)
			telemetry.RecordSpanError(attemptSpan, lastErr)
			slog.ErrorContext(attemptCtx, "fetch attempt failed without retry", "source_name", source.Name, "url", url, "status", status, "error", lastErr)
			attemptSpan.End()
			telemetry.RecordSpanError(span, lastErr)
			return nil, lastErr
		}
		if attempt < retries-1 {
			sleep := time.Duration(1<<attempt) * time.Second
			slog.WarnContext(attemptCtx, "fetch attempt failed; retrying", "source_name", source.Name, "url", url, "status", status, "error", lastErr, "sleep", sleep)
			telemetry.RecordSpanError(attemptSpan, lastErr)
			attemptSpan.End()
			time.Sleep(sleep)
		} else {
			telemetry.RecordSpanError(attemptSpan, lastErr)
			attemptSpan.End()
		}
	}
	telemetry.FetchFailures.Add(ctx, 1)
	telemetry.RecordSpanError(span, lastErr)
	slog.ErrorContext(ctx, "fetch source failed", "source_name", source.Name, "url", url, "error", lastErr)
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
		slog.Info("fetch source using api key auth", "source_name", source.Name)
		headers.Set("apikey", key)
	}
	return headers, nil
}

func fetchOnce(ctx context.Context, client *http.Client, url string, headers http.Header) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

func traceAttrs(values ...any) []trace.SpanStartOption {
	attrs := make([]attribute.KeyValue, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			continue
		}
		switch value := values[i+1].(type) {
		case string:
			attrs = append(attrs, attribute.String(key, value))
		case int:
			attrs = append(attrs, attribute.Int(key, value))
		case bool:
			attrs = append(attrs, attribute.Bool(key, value))
		}
	}
	return []trace.SpanStartOption{trace.WithAttributes(attrs...)}
}

func isRetriable(status int, err error) bool {
	if err != nil {
		return true
	}
	return status == http.StatusTooManyRequests || status >= 500 || errors.Is(err, io.ErrUnexpectedEOF)
}
