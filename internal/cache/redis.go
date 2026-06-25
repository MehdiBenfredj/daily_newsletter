package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MehdiBenfredj/daily_newsletter/internal/telemetry"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	RedisURLEnv         = "REDIS_URL"
	RedisRatingTTLEnv   = "REDIS_RATING_TTL"
	defaultRedisTimeout = 3 * time.Second
	defaultRatingTTL    = 24 * time.Hour
)

var ErrCacheMiss = errors.New("cache miss")

type RatingCache interface {
	Get(ctx context.Context, url string) (float64, error)
	Set(ctx context.Context, url string, rating float64) error
}

type RedisRatingCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisRatingCacheFromEnv() (*RedisRatingCache, bool, error) {
	raw := strings.TrimSpace(os.Getenv(RedisURLEnv))
	if raw == "" {
		slog.Info("rating cache disabled", "env_var", RedisURLEnv)
		return nil, false, nil
	}
	ttl, err := redisRatingTTLFromEnv()
	if err != nil {
		return nil, false, err
	}
	cache, err := NewRedisRatingCacheWithTTL(raw, ttl)
	if err != nil {
		return nil, false, err
	}
	options := cache.client.Options()
	slog.Info("redis rating cache configured", "address", options.Addr, "db", options.DB, "tls", options.TLSConfig != nil)
	return cache, true, nil
}

func NewRedisRatingCache(rawURL string) (*RedisRatingCache, error) {
	return NewRedisRatingCacheWithTTL(rawURL, defaultRatingTTL)
}

func NewRedisRatingCacheWithTTL(rawURL string, ttl time.Duration) (*RedisRatingCache, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", RedisURLEnv, err)
	}
	if !strings.HasPrefix(rawURL, "redis://") && !strings.HasPrefix(rawURL, "rediss://") {
		return nil, fmt.Errorf("%s must use redis:// or rediss://", RedisURLEnv)
	}
	options.DialTimeout = defaultRedisTimeout
	options.ReadTimeout = defaultRedisTimeout
	options.WriteTimeout = defaultRedisTimeout
	return &RedisRatingCache{client: redis.NewClient(options), ttl: ttl}, nil
}

func redisRatingTTLFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(RedisRatingTTLEnv))
	if raw == "" {
		return defaultRatingTTL, nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", RedisRatingTTLEnv, err)
	}
	return ttl, nil
}

func (c *RedisRatingCache) Get(ctx context.Context, key string) (float64, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "rating_cache.get", traceAttrs("cache.key", key)...)
	defer span.End()
	if strings.TrimSpace(key) == "" {
		return 0, ErrCacheMiss
	}
	value, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			span.SetAttributes(attribute.Bool("cache.hit", false))
			return 0, ErrCacheMiss
		}
		telemetry.RecordSpanError(span, err)
		return 0, err
	}
	rating, err := strconv.ParseFloat(value, 64)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return 0, fmt.Errorf("parse cached rating for %q: %w", key, err)
	}
	span.SetAttributes(attribute.Bool("cache.hit", true), attribute.Float64("rating.score", rating))
	return rating, nil
}

func (c *RedisRatingCache) Set(ctx context.Context, key string, rating float64) error {
	ctx, span := telemetry.Tracer().Start(ctx, "rating_cache.set", traceAttrs("cache.key", key, "rating.score", rating)...)
	defer span.End()
	if strings.TrimSpace(key) == "" {
		return nil
	}
	if err := c.client.Set(ctx, key, strconv.FormatFloat(rating, 'g', -1, 64), c.ttl).Err(); err != nil {
		telemetry.RecordSpanError(span, err)
		return err
	}
	return nil
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
		case float64:
			attrs = append(attrs, attribute.Float64(key, value))
		}
	}
	return []trace.SpanStartOption{trace.WithAttributes(attrs...)}
}
