package ratelimit_test

import (
	"testing"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/ratelimit"
)

func TestLimiterAllow(t *testing.T) {
	l := ratelimit.NewLimiter()
	cfg := adapter.RateLimitConfig{
		Requests: 3,
		Per:      time.Second,
	}

	// First 3 requests should be allowed.
	for i := 0; i < 3; i++ {
		ok, _ := l.Allow("test", cfg)
		if !ok {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied.
	ok, wait := l.Allow("test", cfg)
	if ok {
		t.Error("4th request should be denied")
	}
	if wait <= 0 {
		t.Error("expected positive wait duration")
	}
}

func TestLimiterDifferentSources(t *testing.T) {
	l := ratelimit.NewLimiter()
	cfg := adapter.RateLimitConfig{
		Requests: 1,
		Per:      time.Second,
	}

	// First source.
	ok, _ := l.Allow("source1", cfg)
	if !ok {
		t.Error("source1 should be allowed")
	}

	// Second source should also be allowed (different bucket).
	ok, _ = l.Allow("source2", cfg)
	if !ok {
		t.Error("source2 should be allowed")
	}

	// First source again should be denied.
	ok, _ = l.Allow("source1", cfg)
	if ok {
		t.Error("source1 should be denied on second request")
	}
}

func TestFormatRateLimitError(t *testing.T) {
	msg := ratelimit.FormatRateLimitError("test", 30*time.Second)
	if msg == "" {
		t.Error("expected non-empty error message")
	}
}
