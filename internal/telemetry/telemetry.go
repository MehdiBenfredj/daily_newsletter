package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultServiceName = "daily-newsletter-publisher"
	defaultEndpoint    = "localhost:4317"
)

var (
	tracer = otel.Tracer(defaultServiceName)
	meter  = otel.Meter(defaultServiceName)

	SourcesProcessed     metric.Int64Counter
	SourcesFailed        metric.Int64Counter
	ItemsRated           metric.Int64Counter
	ItemsSkipped         metric.Int64Counter
	FetchAttempts        metric.Int64Counter
	FetchFailures        metric.Int64Counter
	OpenRouterRequests   metric.Int64Counter
	OpenRouterLatencyMS  metric.Float64Histogram
	SourceFetchLatencyMS metric.Float64Histogram
)

func init() {
	initMeters()
}

type Providers struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
}

func Init(ctx context.Context) (*Providers, error) {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return nil, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName()),
			attribute.String("deployment.environment", getenvDefault("OTEL_ENVIRONMENT", "local")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx, traceOptions()...)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}
	meterExporter, err := otlpmetricgrpc.New(ctx, metricOptions()...)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}
	logExporter, err := otlploggrpc.New(ctx, logOptions()...)
	if err != nil {
		return nil, fmt.Errorf("create log exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(traceExporter)),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(meterExporter)),
	)
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tracer = otel.Tracer(serviceName())
	meter = otel.Meter(serviceName())
	initMeters()

	return &Providers{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		LoggerProvider: loggerProvider,
	}, nil
}

func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var err error
	if p.LoggerProvider != nil {
		err = errors.Join(err, p.LoggerProvider.Shutdown(ctx))
	}
	if p.MeterProvider != nil {
		err = errors.Join(err, p.MeterProvider.Shutdown(ctx))
	}
	if p.TracerProvider != nil {
		err = errors.Join(err, p.TracerProvider.Shutdown(ctx))
	}
	return err
}

func Tracer() trace.Tracer {
	return tracer
}

func Meter() metric.Meter {
	return meter
}

func RecordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func DurationMillis(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}

func initMeters() {
	var err error
	SourcesProcessed, err = meter.Int64Counter("daily_newsletter.sources.processed")
	handleMetricError(err)
	SourcesFailed, err = meter.Int64Counter("daily_newsletter.sources.failed")
	handleMetricError(err)
	ItemsRated, err = meter.Int64Counter("daily_newsletter.items.rated")
	handleMetricError(err)
	ItemsSkipped, err = meter.Int64Counter("daily_newsletter.items.skipped")
	handleMetricError(err)
	FetchAttempts, err = meter.Int64Counter("daily_newsletter.fetch.attempts")
	handleMetricError(err)
	FetchFailures, err = meter.Int64Counter("daily_newsletter.fetch.failures")
	handleMetricError(err)
	OpenRouterRequests, err = meter.Int64Counter("daily_newsletter.openrouter.requests")
	handleMetricError(err)
	OpenRouterLatencyMS, err = meter.Float64Histogram("daily_newsletter.openrouter.latency_ms")
	handleMetricError(err)
	SourceFetchLatencyMS, err = meter.Float64Histogram("daily_newsletter.fetch.latency_ms")
	handleMetricError(err)
}

func handleMetricError(err error) {
	if err != nil {
		slog.Warn("telemetry metric setup failed", "error", err)
	}
}

func traceOptions() []otlptracegrpc.Option {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(defaultEndpoint), otlptracegrpc.WithInsecure()}
	}
	return nil
}

func metricOptions() []otlpmetricgrpc.Option {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") == "" {
		return []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(defaultEndpoint), otlpmetricgrpc.WithInsecure()}
	}
	return nil
}

func logOptions() []otlploggrpc.Option {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") == "" {
		return []otlploggrpc.Option{otlploggrpc.WithEndpoint(defaultEndpoint), otlploggrpc.WithInsecure()}
	}
	return nil
}

func serviceName() string {
	return getenvDefault("OTEL_SERVICE_NAME", defaultServiceName)
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func TraceAttrs(values ...any) []trace.SpanStartOption {
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
		case float64:
			attrs = append(attrs, attribute.Float64(key, value))
		}
	}
	return []trace.SpanStartOption{trace.WithAttributes(attrs...)}
}
