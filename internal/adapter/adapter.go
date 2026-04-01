package adapter

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel error types for structured error classification.
// Use errors.Is() to check and fmt.Errorf("...: %w", ErrRateLimit) to wrap.
var (
	ErrRateLimit    = errors.New("rate limit")
	ErrAuthRequired = errors.New("auth required")
	ErrInvalidInput = errors.New("invalid input")
)

// NewRateLimitError creates a rate limit error wrapping the sentinel.
func NewRateLimitError(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrRateLimit)
}

// NewAuthRequiredError creates an auth-required error wrapping the sentinel.
func NewAuthRequiredError(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrAuthRequired)
}

// NewInvalidInputError creates an invalid-input error wrapping the sentinel.
func NewInvalidInputError(msg string) error {
	return fmt.Errorf("%s: %w", msg, ErrInvalidInput)
}

// RateLimitConfig defines rate limiting parameters for an adapter.
type RateLimitConfig struct {
	Requests int           `json:"requests" yaml:"requests"`
	Per      time.Duration `json:"per" yaml:"per"`
}

// Valid returns true if the rate limit config has safe, non-zero values.
// A zero Per duration would cause division-by-zero in rate calculations.
func (c RateLimitConfig) Valid() bool {
	return c.Requests > 0 && c.Per > 0
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

// ToolParam describes a single MCP tool parameter for a given endpoint.
type ToolParam struct {
	Name        string // parameter name
	Description string // human-readable description
	Required    bool   // whether the parameter is required
	Default     string // default value (empty if none)
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

	// ToolParams returns MCP tool parameter definitions for a given endpoint.
	// Each adapter self-describes its parameters so the MCP server can
	// register tools without hardcoded switch statements.
	ToolParams(endpoint string) []ToolParam
}
