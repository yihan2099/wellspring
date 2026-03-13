package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/adapter/declarative"
	"github.com/wellspring-cli/wellspring/internal/config"
)

// CatalogEntry represents a single API from the public-apis catalog.
type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Auth        string `json:"auth"`
	HTTPS       bool   `json:"https"`
	CORS        string `json:"cors"`
	Link        string `json:"link"`
	Category    string `json:"category"`
}

// SourceInfo represents information about a source for display.
type SourceInfo struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Auth        string `json:"auth"`
	Supported   bool   `json:"supported"`
	Description string `json:"description"`
	AdapterType string `json:"adapter_type,omitempty"` // "declarative", "coded", or ""
}

// Registry manages all available adapters and the API catalog.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]adapter.Adapter // keyed by name
	catalog  []CatalogEntry
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]adapter.Adapter),
	}
}

// Register adds an adapter to the registry.
func (r *Registry) Register(a adapter.Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Name()] = a
}

// Get retrieves an adapter by name.
func (r *Registry) Get(name string) (adapter.Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	return a, ok
}

// GetByCategory returns all adapters in a given category.
func (r *Registry) GetByCategory(category string) []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []adapter.Adapter
	for _, a := range r.adapters {
		if a.Category() == category {
			result = append(result, a)
		}
	}
	return result
}

// All returns all registered adapters.
func (r *Registry) All() []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]adapter.Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		result = append(result, a)
	}
	return result
}

// Categories returns all unique categories from registered adapters.
func (r *Registry) Categories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cats := make(map[string]bool)
	for _, a := range r.adapters {
		cats[a.Category()] = true
	}
	result := make([]string, 0, len(cats))
	for c := range cats {
		result = append(result, c)
	}
	sort.Strings(result)
	return result
}

// LoadCatalog loads the public-apis catalog from the cache or built-in.
func (r *Registry) LoadCatalog() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Try cached catalog first.
	cacheDir := config.DefaultCacheDir()
	catalogPath := filepath.Join(cacheDir, "catalog.json")
	if data, err := os.ReadFile(catalogPath); err == nil {
		if err := json.Unmarshal(data, &r.catalog); err == nil {
			return nil
		}
	}

	// Use built-in minimal catalog.
	r.catalog = builtInCatalog()
	return nil
}

// SetCatalog sets the catalog entries directly.
func (r *Registry) SetCatalog(entries []CatalogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.catalog = entries
}

// Sources returns information about all known sources (catalog + adapters).
//
// Metadata precedence: when a source exists in both the catalog and as a
// registered adapter, the catalog entry's metadata (Name, Description,
// Category, Auth) takes precedence. This is by design — the catalog is
// curated and may contain richer descriptions than the adapter's built-in
// metadata. Adapters not in the catalog use their own metadata as-is.
func (r *Registry) Sources(filter SourceFilter) []SourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build a map of supported sources.
	supported := make(map[string]adapter.Adapter)
	for name, a := range r.adapters {
		supported[strings.ToLower(name)] = a
	}

	var sources []SourceInfo

	// Add catalog entries first — catalog metadata wins over adapter metadata
	// for sources that exist in both (see precedence note above).
	seen := make(map[string]bool)
	for _, entry := range r.catalog {
		name := strings.ToLower(entry.Name)
		seen[name] = true

		info := SourceInfo{
			Name:        entry.Name,
			Category:    entry.Category,
			Auth:        entry.Auth,
			Description: entry.Description,
		}

		if a, ok := supported[name]; ok {
			info.Supported = true
			if _, isDecl := a.(*declarative.DeclarativeAdapter); isDecl {
				info.AdapterType = "declarative"
			} else {
				info.AdapterType = "coded"
			}
		}

		if filter.matches(info) {
			sources = append(sources, info)
		}
	}

	// Add any registered adapters not in catalog.
	for name, a := range r.adapters {
		if !seen[strings.ToLower(name)] {
			info := SourceInfo{
				Name:        a.Name(),
				Category:    a.Category(),
				Auth:        "none",
				Supported:   true,
				Description: a.Description(),
			}
			if _, isDecl := a.(*declarative.DeclarativeAdapter); isDecl {
				info.AdapterType = "declarative"
			} else {
				info.AdapterType = "coded"
			}
			if !a.RequiresAuth() {
				info.Auth = "none"
			} else {
				info.Auth = "apiKey"
			}
			if filter.matches(info) {
				sources = append(sources, info)
			}
		}
	}

	sort.Slice(sources, func(i, j int) bool {
		// Supported first, then alphabetical.
		if sources[i].Supported != sources[j].Supported {
			return sources[i].Supported
		}
		return sources[i].Name < sources[j].Name
	})

	return sources
}

// SourceFilter controls which sources to return.
type SourceFilter struct {
	Category    string
	Auth        string
	Supported   *bool // nil = all, true = supported only, false = unsupported only
}

func (f SourceFilter) matches(info SourceInfo) bool {
	if f.Category != "" && !strings.EqualFold(info.Category, f.Category) {
		return false
	}
	if f.Auth != "" && !strings.EqualFold(info.Auth, f.Auth) {
		return false
	}
	if f.Supported != nil && info.Supported != *f.Supported {
		return false
	}
	return true
}

// LoadUserSources loads user-defined YAML sources from the config directory.
func (r *Registry) LoadUserSources() error {
	dir := config.UserSourcesDir()
	adapters, err := declarative.LoadAllFromDir(dir)
	if err != nil {
		return err
	}
	for _, a := range adapters {
		r.Register(a)
	}
	return nil
}

// SaveCatalog saves the catalog to the cache directory.
func (r *Registry) SaveCatalog(entries []CatalogEntry) error {
	cacheDir := config.DefaultCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(cacheDir, "catalog.json"), data, 0o644)
}

// builtInCatalog returns a minimal built-in catalog with known APIs.
func builtInCatalog() []CatalogEntry {
	return []CatalogEntry{
		{Name: "hackernews", Description: "Hacker News API", Auth: "", Category: "News", Link: "https://github.com/HackerNews/API"},
		{Name: "openmeteo", Description: "Open-Meteo Weather API", Auth: "", Category: "Weather", Link: "https://open-meteo.com/"},
		{Name: "reddit", Description: "Reddit JSON API", Auth: "", Category: "News", Link: "https://www.reddit.com/dev/api/"},
		{Name: "coingecko", Description: "CoinGecko Crypto API", Auth: "", Category: "Cryptocurrency", Link: "https://www.coingecko.com/en/api"},
		{Name: "alphavantage", Description: "Alpha Vantage Financial Data", Auth: "apiKey", Category: "Finance", Link: "https://www.alphavantage.co/"},
		{Name: "worldbank", Description: "World Bank Open Data", Auth: "", Category: "Government", Link: "https://data.worldbank.org/"},
		{Name: "Dev.to", Description: "Dev.to API", Auth: "", Category: "News", Link: "https://developers.forem.com/api/"},
		{Name: "News API", Description: "News aggregation API", Auth: "apiKey", Category: "News", Link: "https://newsapi.org/"},
		{Name: "The Guardian", Description: "Guardian News API", Auth: "apiKey", Category: "News", Link: "https://open-platform.theguardian.com/"},
		{Name: "GNews", Description: "GNews API", Auth: "apiKey", Category: "News", Link: "https://gnews.io/"},
		{Name: "Currents", Description: "Currents API", Auth: "apiKey", Category: "News", Link: "https://currentsapi.services/"},
		{Name: "wttr.in", Description: "Weather in terminal", Auth: "", Category: "Weather", Link: "https://wttr.in/"},
		{Name: "OpenWeatherMap", Description: "OpenWeather API", Auth: "apiKey", Category: "Weather", Link: "https://openweathermap.org/api"},
		{Name: "CoinMarketCap", Description: "CoinMarketCap API", Auth: "apiKey", Category: "Cryptocurrency", Link: "https://coinmarketcap.com/api/"},
		{Name: "Binance", Description: "Binance Exchange API", Auth: "apiKey", Category: "Cryptocurrency", Link: "https://binance-docs.github.io/apidocs/"},
		{Name: "Yahoo Finance", Description: "Yahoo Finance API", Auth: "", Category: "Finance", Link: "https://finance.yahoo.com/"},
		{Name: "IEX Cloud", Description: "IEX Cloud Financial Data", Auth: "apiKey", Category: "Finance", Link: "https://iexcloud.io/docs/"},
		{Name: "NASA", Description: "NASA Open APIs", Auth: "", Category: "Science & Math", Link: "https://api.nasa.gov/"},
		{Name: "REST Countries", Description: "Country Data API", Auth: "", Category: "Government", Link: "https://restcountries.com/"},
		{Name: "Data.gov", Description: "US Government Open Data", Auth: "apiKey", Category: "Government", Link: "https://api.data.gov/"},
	}
}

// ParsePublicAPIsREADME parses the public-apis README format to extract catalog entries.
func ParsePublicAPIsREADME(content string) []CatalogEntry {
	var entries []CatalogEntry
	var currentCategory string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Category headers: ### Category Name
		if strings.HasPrefix(line, "### ") {
			currentCategory = strings.TrimPrefix(line, "### ")
			continue
		}

		// Table rows: | Name | Description | Auth | HTTPS | CORS | Link |
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "| API") || strings.Contains(line, "---") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			continue
		}

		// Clean up parts.
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}

		// Extract link from markdown: [Name](url)
		name := parts[1]
		link := ""
		if idx := strings.Index(name, "]("); idx != -1 {
			link = name[idx+2:]
			if end := strings.Index(link, ")"); end != -1 {
				link = link[:end]
			}
			name = name[1:idx]
		}

		auth := parts[3]
		if auth == "No" || auth == "" {
			auth = ""
		}

		httpsStr := parts[4]
		isHTTPS := strings.EqualFold(httpsStr, "Yes")

		entry := CatalogEntry{
			Name:        name,
			Description: parts[2],
			Auth:        auth,
			HTTPS:       isHTTPS,
			CORS:        parts[5],
			Link:        link,
			Category:    currentCategory,
		}
		entries = append(entries, entry)
	}

	return entries
}

// FormatSourcesOutput formats the sources list for display.
func FormatSourcesOutput(sources []SourceInfo) string {
	if len(sources) == 0 {
		return "No sources found matching your criteria."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %-20s %-10s %s\n", "Source", "Auth", "Status"))
	b.WriteString("  " + strings.Repeat("─", 50) + "\n")

	supportedCount := 0
	availableCount := 0

	for _, s := range sources {
		status := "no adapter"
		marker := " "
		if s.Supported {
			status = "supported"
			marker = "✓"
			supportedCount++
		} else {
			availableCount++
		}

		auth := s.Auth
		if auth == "" {
			auth = "none"
		}

		b.WriteString(fmt.Sprintf("%s %-20s %-10s %s\n", marker, s.Name, auth, status))
	}

	b.WriteString(fmt.Sprintf("\n  %d supported · %d available · PRs welcome → github.com/wellspring-cli/registry\n",
		supportedCount, availableCount))

	return b.String()
}
