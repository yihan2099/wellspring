package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/config"
)

// Limiter tracks per-source rate limits using a simple sliding window.
type Limiter struct {
	mu       sync.Mutex
	windows  map[string][]time.Time
}

// NewLimiter creates a new rate limiter.
func NewLimiter() *Limiter {
	return &Limiter{
		windows: make(map[string][]time.Time),
	}
}

// Allow checks if a request to the given source is allowed.
// Returns true if allowed, false if rate limited, and the wait duration.
func (l *Limiter) Allow(source string, cfg adapter.RateLimitConfig) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	window := l.windows[source]

	// Remove expired entries.
	cutoff := now.Add(-cfg.Per)
	valid := make([]time.Time, 0, len(window))
	for _, t := range window {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= cfg.Requests {
		// Rate limited — calculate wait time.
		oldest := valid[0]
		waitUntil := oldest.Add(cfg.Per)
		return false, waitUntil.Sub(now)
	}

	// Allowed — record this request.
	valid = append(valid, now)
	l.windows[source] = valid
	return true, 0
}

// --- Response Cache ---

// CacheEntry stores a cached API response.
type CacheEntry struct {
	Points    []adapter.DataPoint `json:"points"`
	CachedAt  time.Time           `json:"cached_at"`
	ExpiresAt time.Time           `json:"expires_at"`
}

// Cache provides file-based response caching.
type Cache struct {
	dir string
	ttl time.Duration
}

// NewCache creates a new file-based cache.
func NewCache(ttl time.Duration) *Cache {
	dir := filepath.Join(config.DefaultCacheDir(), "responses")
	os.MkdirAll(dir, 0o755)
	return &Cache{
		dir: dir,
		ttl: ttl,
	}
}

// cacheKey generates a deterministic cache key from source + params.
func cacheKey(source string, params map[string]string) string {
	h := sha256.New()
	h.Write([]byte(source))

	// Sort params for deterministic key.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(params[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Get retrieves a cached response if available and not expired.
func (c *Cache) Get(source string, params map[string]string) ([]adapter.DataPoint, bool) {
	key := cacheKey(source, params)
	path := filepath.Join(c.dir, key+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		os.Remove(path) // Clean up expired entry.
		return nil, false
	}

	return entry.Points, true
}

// Set stores a response in the cache.
func (c *Cache) Set(source string, params map[string]string, points []adapter.DataPoint) {
	key := cacheKey(source, params)
	path := filepath.Join(c.dir, key+".json")

	entry := CacheEntry{
		Points:    points,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(c.ttl),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	os.MkdirAll(c.dir, 0o755)
	os.WriteFile(path, data, 0o644)
}

// Clear removes all cached responses.
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		os.Remove(filepath.Join(c.dir, e.Name()))
	}
	return nil
}

// Status returns cache statistics.
func (c *Cache) Status() (total int, expired int) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0, 0
	}
	now := time.Now()
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		total++
		data, err := os.ReadFile(filepath.Join(c.dir, e.Name()))
		if err != nil {
			continue
		}
		var entry CacheEntry
		if json.Unmarshal(data, &entry) == nil && now.After(entry.ExpiresAt) {
			expired++
		}
	}
	return total, expired
}

// FormatRateLimitError returns a user-friendly rate limit error message.
func FormatRateLimitError(source string, wait time.Duration) string {
	return fmt.Sprintf("rate limited by %s — try again in %s\n\nHint: use --cache to serve from cache, or wait for the rate limit window to reset", source, wait.Round(time.Second))
}
