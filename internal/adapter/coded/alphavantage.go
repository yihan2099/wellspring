package coded

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
)

// AlphaVantageAdapter implements adapter.Adapter for the Alpha Vantage API.
// Alpha Vantage requires an API key and has strict rate limits (5 req/min for free tier).
type AlphaVantageAdapter struct {
	client *http.Client
	apiKey string
}

// NewAlphaVantageAdapter creates a new Alpha Vantage adapter.
func NewAlphaVantageAdapter() *AlphaVantageAdapter {
	// Try to get API key from environment.
	apiKey := os.Getenv("WSP_ALPHA_VANTAGE_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ALPHA_VANTAGE_API_KEY")
	}

	return &AlphaVantageAdapter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey: apiKey,
	}
}

func (a *AlphaVantageAdapter) Name() string        { return "alphavantage" }
func (a *AlphaVantageAdapter) Category() string     { return "finance" }
func (a *AlphaVantageAdapter) Description() string  { return "Alpha Vantage — stock quotes, time series, technical indicators" }
func (a *AlphaVantageAdapter) RequiresAuth() bool   { return true }
func (a *AlphaVantageAdapter) Endpoints() []string  { return []string{"quote", "daily", "search"} }

func (a *AlphaVantageAdapter) RateLimit() adapter.RateLimitConfig {
	return adapter.RateLimitConfig{
		Requests: 5,
		Per:      time.Minute,
	}
}

func (a *AlphaVantageAdapter) getAPIKey(params map[string]string) string {
	if key, ok := params["api_key"]; ok && key != "" {
		return key
	}
	return a.apiKey
}

func (a *AlphaVantageAdapter) Fetch(ctx context.Context, params map[string]string) ([]adapter.DataPoint, error) {
	apiKey := a.getAPIKey(params)
	if apiKey == "" {
		return nil, fmt.Errorf("Alpha Vantage requires an API key\n\n" +
			"Set it via:\n" +
			"  export WSP_ALPHA_VANTAGE_KEY=your_key\n" +
			"  or in ~/.config/wellspring/config.toml under [keys]\n\n" +
			"Get a free key at: https://www.alphavantage.co/support/#api-key")
	}

	action := params["action"]
	if action == "" {
		action = "quote"
	}

	switch action {
	case "quote":
		return a.fetchQuote(ctx, params, apiKey)
	case "daily":
		return a.fetchDaily(ctx, params, apiKey)
	case "search":
		return a.fetchSearch(ctx, params, apiKey)
	default:
		return nil, fmt.Errorf("unknown action %q for alphavantage (available: quote, daily, search)", action)
	}
}

func (a *AlphaVantageAdapter) fetchQuote(ctx context.Context, params map[string]string, apiKey string) ([]adapter.DataPoint, error) {
	symbol := params["symbol"]
	if symbol == "" {
		return nil, fmt.Errorf("--symbol is required for stock quotes\n\nExample: wsp finance quote --symbol=AAPL")
	}

	url := fmt.Sprintf("https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s&apikey=%s",
		strings.ToUpper(symbol), apiKey)

	body, err := a.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Check for rate limit message.
	if note, ok := result["Note"]; ok {
		return nil, fmt.Errorf("rate limited by Alpha Vantage: %v\n\nHint: free tier allows 5 requests/minute", note)
	}
	if info, ok := result["Information"]; ok {
		return nil, fmt.Errorf("Alpha Vantage: %v", info)
	}

	quote, ok := result["Global Quote"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format from Alpha Vantage")
	}

	price := getFloat(quote, "05. price")
	change := getFloat(quote, "09. change")
	changePct := getString(quote, "10. change percent")

	dp := adapter.DataPoint{
		Source:   "alphavantage",
		Category: "finance",
		Title:   strings.ToUpper(symbol),
		Value:   price,
		Time:    time.Now(),
		URL:     fmt.Sprintf("https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s", symbol),
		Meta: map[string]any{
			"symbol":     getString(quote, "01. symbol"),
			"open":       getFloat(quote, "02. open"),
			"high":       getFloat(quote, "03. high"),
			"low":        getFloat(quote, "04. low"),
			"volume":     getString(quote, "06. volume"),
			"change":     change,
			"change_pct": changePct,
			"prev_close": getFloat(quote, "08. previous close"),
		},
	}

	return []adapter.DataPoint{dp}, nil
}

func (a *AlphaVantageAdapter) fetchDaily(ctx context.Context, params map[string]string, apiKey string) ([]adapter.DataPoint, error) {
	symbol := params["symbol"]
	if symbol == "" {
		return nil, fmt.Errorf("--symbol is required for daily time series\n\nExample: wsp finance daily --symbol=AAPL")
	}

	limit := 10
	if l, ok := params["limit"]; ok {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}

	outputsize := "compact"
	if limit > 100 {
		outputsize = "full"
	}

	url := fmt.Sprintf("https://www.alphavantage.co/query?function=TIME_SERIES_DAILY&symbol=%s&outputsize=%s&apikey=%s",
		strings.ToUpper(symbol), outputsize, apiKey)

	body, err := a.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if note, ok := result["Note"]; ok {
		return nil, fmt.Errorf("rate limited by Alpha Vantage: %v", note)
	}

	timeSeries, ok := result["Time Series (Daily)"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format from Alpha Vantage")
	}

	// Sort dates descending.
	dates := make([]string, 0, len(timeSeries))
	for date := range timeSeries {
		dates = append(dates, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	if limit > 0 && len(dates) > limit {
		dates = dates[:limit]
	}

	points := make([]adapter.DataPoint, 0, len(dates))
	for _, date := range dates {
		day := timeSeries[date].(map[string]any)
		t, _ := time.Parse("2006-01-02", date)

		dp := adapter.DataPoint{
			Source:   "alphavantage",
			Category: "finance",
			Title:   fmt.Sprintf("%s %s", strings.ToUpper(symbol), date),
			Value:   getFloat(day, "4. close"),
			Time:    t,
			Meta: map[string]any{
				"symbol": strings.ToUpper(symbol),
				"open":   getFloat(day, "1. open"),
				"high":   getFloat(day, "2. high"),
				"low":    getFloat(day, "3. low"),
				"close":  getFloat(day, "4. close"),
				"volume": getString(day, "5. volume"),
			},
		}
		points = append(points, dp)
	}

	return points, nil
}

func (a *AlphaVantageAdapter) fetchSearch(ctx context.Context, params map[string]string, apiKey string) ([]adapter.DataPoint, error) {
	query := params["query"]
	if query == "" {
		query = params["symbol"]
	}
	if query == "" {
		return nil, fmt.Errorf("--query or --symbol is required for search\n\nExample: wsp finance search --query=Apple")
	}

	url := fmt.Sprintf("https://www.alphavantage.co/query?function=SYMBOL_SEARCH&keywords=%s&apikey=%s",
		query, apiKey)

	body, err := a.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	matches, ok := result["bestMatches"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	limit := 10
	if l, ok := params["limit"]; ok {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}

	points := make([]adapter.DataPoint, 0, len(matches))
	for _, m := range matches {
		match := m.(map[string]any)
		dp := adapter.DataPoint{
			Source:   "alphavantage",
			Category: "finance",
			Title:   getString(match, "2. name"),
			Value:   getString(match, "9. matchScore"),
			Time:    time.Now(),
			Meta: map[string]any{
				"symbol":   getString(match, "1. symbol"),
				"type":     getString(match, "3. type"),
				"region":   getString(match, "4. region"),
				"currency": getString(match, "8. currency"),
			},
		}
		points = append(points, dp)
	}

	return points, nil
}

func (a *AlphaVantageAdapter) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "wellspring-cli/0.1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Alpha Vantage returned status %d: %s", resp.StatusCode, truncateBody(string(body)))
	}

	return io.ReadAll(resp.Body)
}

// Helper to get a float from a map with a string value.
func getFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

// Helper to get a string from a map.
func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
