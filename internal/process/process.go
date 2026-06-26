package process

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/fetch"
	"github.com/MehdiBenfredj/daily_newsletter/internal/parse"
	"github.com/MehdiBenfredj/daily_newsletter/internal/telemetry"
	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
	"go.opentelemetry.io/otel/attribute"
)

func ProcessSources(ctx context.Context, collection types.Collection) ([]types.ProcessedSource, []types.ProcessedSource) {
	ctx, span := telemetry.Tracer().Start(ctx, "sources.process", telemetry.TraceAttrs("sources", len(collection.Sources))...)
	defer span.End()
	processed := make([]types.ProcessedSource, 0, len(collection.Sources))
	errored := make([]types.ProcessedSource, 0, len(collection.Sources))
	now := time.Now().UTC()
	for _, source := range collection.Sources {
		sourceCtx, sourceSpan := telemetry.Tracer().Start(ctx, "source.process", telemetry.TraceAttrs("source.name", source.Name, "source.url", source.URL, "source.theme", source.Theme, "source.type", source.Type)...)
		slog.InfoContext(sourceCtx, "processing source", "source_name", source.Name, "url", source.URL, "theme", source.Theme, "type", source.Type)
		item := types.ProcessedSource{
			Theme:              source.Theme,
			Name:               source.Name,
			URL:                source.URL,
			Type:               source.Type,
			Config:             source.Config,
			PersonalPreference: source.PersonalPreference,
		}
		result, err := ProcessSource(sourceCtx, source)
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			errored = append(errored, item)
			telemetry.RecordSpanError(sourceSpan, err)
			telemetry.SourcesFailed.Add(sourceCtx, 1)
			slog.ErrorContext(sourceCtx, "source processing failed", "source_name", source.Name, "url", source.URL, "error", err)
			sourceSpan.End()
			continue
		}
		item.OK = true
		item.Processed = &result
		sourceSpan.SetAttributes(attribute.String("content.type", result.ContentType), attribute.Int("content.bytes", result.Bytes))
		slog.InfoContext(sourceCtx, "source fetched", "source_name", source.Name, "content_type", result.ContentType, "bytes", result.Bytes)
		parseCtx, parseSpan := telemetry.Tracer().Start(sourceCtx, "source.parse", telemetry.TraceAttrs("source.name", source.Name, "content.type", result.ContentType)...)
		info, err := parse.ProcessedSource(item, now)
		if err != nil {
			item.ParseError = err.Error()
			telemetry.RecordSpanError(parseSpan, err)
			slog.ErrorContext(parseCtx, "source parsing failed", "source_name", source.Name, "error", err)
		} else {
			item.Info = info
			parseSpan.SetAttributes(attribute.Int("items", len(info)))
			slog.InfoContext(parseCtx, "source parsed", "source_name", source.Name, "items", len(info))
		}
		parseSpan.End()
		telemetry.SourcesProcessed.Add(sourceCtx, 1)
		sourceSpan.SetAttributes(attribute.Int("items", len(item.Info)))
		sourceSpan.End()
		processed = append(processed, item)
	}
	span.SetAttributes(attribute.Int("processed", len(processed)), attribute.Int("errored", len(errored)))
	return processed, errored
}

func ProcessSource(ctx context.Context, source types.Source) (types.Processed, error) {
	slog.InfoContext(ctx, "fetching source", "source_name", source.Name, "url", source.URL, "type", source.Type)
	raw, err := fetch.SourceContext(ctx, source)
	if err != nil {
		return types.Processed{}, err
	}
	sourceType := strings.ToLower(source.Type)
	if sourceType == "" {
		sourceType = "rss"
	}
	text := string(raw)
	switch sourceType {
	case "rss", "feed", "atom", "xml":
		return types.Processed{ContentType: "rss", Bytes: len(raw), Data: text}, nil
	case "website", "html", "web":
		return types.Processed{ContentType: "website", Bytes: len(raw), Data: text}, nil
	case "api":
		var data any
		if err := json.Unmarshal(raw, &data); err != nil {
			data = text
		}
		return types.Processed{ContentType: "api", Bytes: len(raw), Data: data}, nil
	default:
		return types.Processed{}, fmt.Errorf("unsupported source type %q for %s", sourceType, source.Name)
	}
}
