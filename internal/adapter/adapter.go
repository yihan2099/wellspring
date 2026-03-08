package adapter

import (
	"context"
	"time"
)

// RateLimitConfig defines rate limiting parameters for an adapter.
type RateLimitConfig struct {
	Requests int           `json:"requests" yaml:"requests"`
	Per      time.Duration `json:"per" yaml:"per"`
}

// DataPoint is the normalized output format for all adapters.
type DataPoint struct {
	Source   string         `json:"source"`
	Category string        `json:"category"`
	Time    time.Time      `json:"time"`
	Title   string         `json:"title,omitempty"`
	Value   any            `json:"value,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
	URL     string         `json:"url,omitempty"`
}

// Adapter is the interface that all data source adapters must implement.
type Adapter interface {
	// Name returns the adapter's unique name (e.g., "hackernews").
	Name() string

	// Category returns the adapter's category (e.g., "news").
	Category() string

	// RequiresAuth returns whether the adapter requires authentication.
	RequiresAuth() bool

	// Fetch retrieves data from the source with the given parameters.
	Fetch(ctx context.Context, params map[string]string) ([]DataPoint, error)

	// RateLimit returns the rate limiting configuration for this adapter.
	RateLimit() RateLimitConfig

	// Endpoints returns the list of available endpoint/action names.
	Endpoints() []string

	// Description returns a human-readable description of the adapter.
	Description() string
}
