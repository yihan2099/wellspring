package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/registry"
)

// mockAdapter implements adapter.Adapter for testing.
type mockAdapter struct {
	name     string
	category string
	auth     bool
}

func (m *mockAdapter) Name() string        { return m.name }
func (m *mockAdapter) Category() string    { return m.category }
func (m *mockAdapter) RequiresAuth() bool  { return m.auth }
func (m *mockAdapter) Description() string { return "mock " + m.name }
func (m *mockAdapter) Endpoints() []string { return []string{"list"} }
func (m *mockAdapter) RateLimit() adapter.RateLimitConfig {
	return adapter.RateLimitConfig{Requests: 10, Per: time.Minute}
}
func (m *mockAdapter) Fetch(ctx context.Context, params map[string]string) ([]adapter.DataPoint, error) {
	return nil, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := registry.NewRegistry()

	mock := &mockAdapter{name: "test", category: "news"}
	reg.Register(mock)

	a, ok := reg.Get("test")
	if !ok {
		t.Fatal("expected to find adapter 'test'")
	}
	if a.Name() != "test" {
		t.Errorf("expected name 'test', got %q", a.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	reg := registry.NewRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent adapter")
	}
}

func TestRegistryGetByCategory(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(&mockAdapter{name: "hn", category: "news"})
	reg.Register(&mockAdapter{name: "reddit", category: "news"})
	reg.Register(&mockAdapter{name: "weather", category: "weather"})

	news := reg.GetByCategory("news")
	if len(news) != 2 {
		t.Errorf("expected 2 news adapters, got %d", len(news))
	}

	weather := reg.GetByCategory("weather")
	if len(weather) != 1 {
		t.Errorf("expected 1 weather adapter, got %d", len(weather))
	}
}

func TestRegistryCategories(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(&mockAdapter{name: "hn", category: "news"})
	reg.Register(&mockAdapter{name: "weather", category: "weather"})

	cats := reg.Categories()
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}

func TestRegistrySourcesFilter(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(&mockAdapter{name: "hackernews", category: "news"})
	reg.Register(&mockAdapter{name: "reddit", category: "news"})
	reg.LoadCatalog()

	// All sources.
	all := reg.Sources(registry.SourceFilter{})
	if len(all) == 0 {
		t.Error("expected some sources")
	}

	// Supported only.
	supported := true
	supFilter := reg.Sources(registry.SourceFilter{Supported: &supported})
	for _, s := range supFilter {
		if !s.Supported {
			t.Errorf("expected all sources to be supported, got %q unsupported", s.Name)
		}
	}
}

func TestParsePublicAPIsREADME(t *testing.T) {
	content := `### News
| API | Description | Auth | HTTPS | CORS |
|---|---|---|---|---|
| [Hacker News](https://github.com/HackerNews/API) | Hacker News API | No | Yes | Unknown |
| [News API](https://newsapi.org/) | News aggregation | apiKey | Yes | Unknown |

### Weather
| API | Description | Auth | HTTPS | CORS |
|---|---|---|---|---|
| [Open-Meteo](https://open-meteo.com/) | Weather forecasts | No | Yes | Yes |
`

	entries := registry.ParsePublicAPIsREADME(content)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Check first entry.
	if entries[0].Name != "Hacker News" {
		t.Errorf("expected 'Hacker News', got %q", entries[0].Name)
	}
	if entries[0].Category != "News" {
		t.Errorf("expected category 'News', got %q", entries[0].Category)
	}
	if entries[0].Link != "https://github.com/HackerNews/API" {
		t.Errorf("expected link, got %q", entries[0].Link)
	}
}

func TestFormatSourcesOutput(t *testing.T) {
	sources := []registry.SourceInfo{
		{Name: "hackernews", Category: "news", Auth: "none", Supported: true},
		{Name: "Dev.to", Category: "news", Auth: "none", Supported: false},
	}

	out := registry.FormatSourcesOutput(sources)
	if out == "" {
		t.Error("expected non-empty output")
	}
}
