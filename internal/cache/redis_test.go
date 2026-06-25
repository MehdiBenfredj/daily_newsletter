package cache

import (
	"testing"
	"time"
)

func TestNewRedisRatingCacheParsesURL(t *testing.T) {
	cache, err := NewRedisRatingCache("redis://:secret@localhost:6380/2")
	if err != nil {
		t.Fatal(err)
	}
	options := cache.client.Options()
	if options.Addr != "localhost:6380" {
		t.Fatalf("address = %q, want localhost:6380", options.Addr)
	}
	if options.Password != "secret" {
		t.Fatalf("password = %q, want secret", options.Password)
	}
	if options.DB != 2 {
		t.Fatalf("db = %d, want 2", options.DB)
	}
	if options.TLSConfig != nil {
		t.Fatalf("tls = true, want false")
	}
	if options.DialTimeout != defaultRedisTimeout {
		t.Fatalf("dial timeout = %s, want %s", options.DialTimeout, defaultRedisTimeout)
	}
	if cache.ttl != defaultRatingTTL {
		t.Fatalf("ttl = %s, want %s", cache.ttl, defaultRatingTTL)
	}
}

func TestRedisRatingTTLFromEnv(t *testing.T) {
	t.Setenv(RedisRatingTTLEnv, "2h")

	ttl, err := redisRatingTTLFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if ttl != 2*time.Hour {
		t.Fatalf("ttl = %s, want 2h", ttl)
	}
}

func TestRedisRatingTTLFromEnvDefaultsTo24Hours(t *testing.T) {
	t.Setenv(RedisRatingTTLEnv, "")

	ttl, err := redisRatingTTLFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if ttl != 24*time.Hour {
		t.Fatalf("ttl = %s, want 24h", ttl)
	}
}
