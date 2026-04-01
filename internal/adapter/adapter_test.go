package adapter_test

import (
	"testing"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
)

func TestDataPointJSON(t *testing.T) {
	dp := adapter.DataPoint{
		Source:   "test",
		Category: "news",
		Time:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Title:   "Test Title",
		Value:   42,
		URL:     "https://example.com",
		Meta:    map[string]any{"key": "value"},
	}

	if dp.Source != "test" {
		t.Errorf("expected source 'test', got %q", dp.Source)
	}
	if dp.Category != "news" {
		t.Errorf("expected category 'news', got %q", dp.Category)
	}
	if dp.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", dp.Title)
	}
	if dp.Value != 42 {
		t.Errorf("expected value 42, got %v", dp.Value)
	}
	if dp.URL != "https://example.com" {
		t.Errorf("expected URL, got %q", dp.URL)
	}
}

func TestRateLimitConfig(t *testing.T) {
	cfg := adapter.RateLimitConfig{
		Requests: 30,
		Per:      time.Minute,
	}

	if cfg.Requests != 30 {
		t.Errorf("expected 30 requests, got %d", cfg.Requests)
	}
	if cfg.Per != time.Minute {
		t.Errorf("expected 1m duration, got %v", cfg.Per)
	}
}
