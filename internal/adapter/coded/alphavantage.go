package coded

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// The apiKey parameter should be resolved by the caller via config.GetAPIKey("ALPHA_VANTAGE"),
// which handles env var lookup (WSP_ALPHA_VANTAGE_KEY, ALPHA_VANTAGE_API_KEY) and config file
// fallback in a single, auditable code path.
func NewAlphaVantageAdapter(apiKey string) *AlphaVantageAdapter {
	return &AlphaVantageAdapter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey: apiKey,
	}
}

func (a *AlphaVantageAdapter) Name() string        { return "alphavantage" }
func (a *AlphaVantageAdapter) Category() string     { return "finance" }
func (a *AlphaVantageAdapter) Description() string  { return "Alpha Vantage — stock quotes, daily time series, symbol search" }
func (a *AlphaVantageAdapter) RequiresAuth() bool   { return true }
func (a *AlphaVantageAdapter) Endpoints() []string  { return []string{"quote", "daily", "search"} }

func (a *AlphaVantageAdapter) RateLimit() adapter.RateLimitConfig {
	return adapter.RateLimitConfig{
		Requests: 5,
		Per:      time.Minute,
	}
}

func (a *AlphaVantageAdapter) ToolParams(endpoint string) []adapter.ToolParam {
	switch endpoint {
	case "search":
		return []adapter.ToolParam{
			{Name: "query", Description: "Search query (company name or partial symbol)", Required: true},
			{Name: "symbol", Description: "Alias for query (stock ticker symbol)"},
			{Name: "limit", Description: "Maximum number of results", Default: "10"},
		}
	default: // quote, daily
		return []adapter.ToolParam{
			{Name: "symbol", Description: "Stock ticker symbol", Required: true},
			{Name: "limit", Description: "Maximum number of results", Default: "10"},
		}
	}
}

func (a *AlphaVantageAdapter) getAPIKey(params map[string]string) string {
	if key, ok := params["api_key"]; ok && key != "" {
		return key
	}
	return a.apiKey
}

func (a *AlphaVantageAdapter) Fetch(ctx context.Context, params map[string]string) ([]adapter.DataPoint, error) {
	if params == nil {
		params = make(map[string]string)
	}
	apiKey := a.getAPIKey(params)
	if apiKey == "" {
		return nil, adapter.NewAuthRequiredError("Alpha Vantage requires an API key\n\n" +
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
		return nil, adapter.NewInvalidInputError(fmt.Sprintf("unknown action %q for alphavantage (available: quote, daily, search)", action))
	}
}

func (a *AlphaVantageAdapter) fetchQuote(ctx context.Context, params map[string]string, apiKey string) ([]adapter.DataPoint, error) {
	symbol := params["symbol"]
	if symbol == "" {
		return nil, adapter.NewInvalidInputError("--symbol is required for stock quotes\n\nExample: wsp finance quote --symbol=AAPL")
	}

	params_ := url.Values{}
	params_.Set("function", "GLOBAL_QUOTE")
	params_.Set("symbol", strings.ToUpper(symbol))
	reqURL := "https://www.alphavantage.co/query?" + params_.Encode()

	body, err := a.doRequest(ctx, reqURL, apiKey)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if err := checkAVError(result); err != nil {
		return nil, err
	}

	quote, ok := result["Global Quote"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format from Alpha Vantage")
	}

	// Track missing fields — a zero price is indistinguishable from a missing
	// field without this check. Consumers can inspect _missing_fields in Meta.
	var missing []string
	expectedKeys := []string{"05. price", "01. symbol", "02. open", "03. high", "04. low", "09. change", "10. change percent"}
	for _, k := range expectedKeys {
		if _, ok := quote[k]; !ok {
			missing = append(missing, k)
		}
	}

	price := getFloat(quote, "05. price")
	change := getFloat(quote, "09. change")
	changePct := getString(quote, "10. change percent")

	meta := map[string]any{
		"symbol":     getString(quote, "01. symbol"),
		"open":       getFloat(quote, "02. open"),
		"high":       getFloat(quote, "03. high"),
		"low":        getFloat(quote, "04. low"),
		"volume":     getString(quote, "06. volume"),
		"change":     change,
		"change_pct": changePct,
		"prev_close": getFloat(quote, "08. previous close"),
	}
	if len(missing) > 0 {
		meta["_missing_fields"] = missing
	}

	dp := adapter.DataPoint{
		Source:   "alphavantage",
		Category: "finance",
		Title:   strings.ToUpper(symbol),
		Value:   price,
		Time:    time.Now(),
		URL:     fmt.Sprintf("https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s", symbol),
		Meta:    meta,
	}

	return []adapter.DataPoint{dp}, nil
}

func (a *AlphaVantageAdapter) fetchDaily(ctx context.Context, params map[string]string, apiKey string) ([]adapter.DataPoint, error) {
	symbol := params["symbol"]
	if symbol == "" {
		return nil, adapter.NewInvalidInputError("--symbol is required for daily time series\n\nExample: wsp finance daily --symbol=AAPL")
	}

	limit := 10
	if l, ok := params["limit"]; ok {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}

	outputsize := "compact"
	if limit > 100 {
		outputsize = "full"
	}

	params_ := url.Values{}
	params_.Set("function", "TIME_SERIES_DAILY")
	params_.Set("symbol", strings.ToUpper(symbol))
	params_.Set("outputsize", outputsize)
	reqURL := "https://www.alphavantage.co/query?" + params_.Encode()

	body, err := a.doRequest(ctx, reqURL, apiKey)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if err := checkAVError(result); err != nil {
		return nil, err
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
		return nil, adapter.NewInvalidInputError("--query or --symbol is required for search\n\nExample: wsp finance search --query=Apple")
	}

	params_ := url.Values{}
	params_.Set("function", "SYMBOL_SEARCH")
	params_.Set("keywords", query)
	reqURL := "https://www.alphavantage.co/query?" + params_.Encode()

	body, err := a.doRequest(ctx, reqURL, apiKey)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if err := checkAVError(result); err != nil {
		return nil, err
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
	if limit < 1 {
		limit = 1
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

func (a *AlphaVantageAdapter) doRequest(ctx context.Context, baseURL string, apiKey string) ([]byte, error) {
	// Add API key to URL at request time only — it never appears in the baseURL
	// string, so log messages, error traces, and caller code cannot leak it.
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	q := u.Query()
	q.Set("apikey", apiKey)
	u.RawQuery = q.Encode()

	// maskErr wraps both the apikey= pattern masking and literal key replacement
	// to ensure the API key never appears in any returned error, regardless of
	// how the underlying HTTP library formats it.
	maskErr := func(err error) error {
		return maskAPIKey(fmt.Errorf("%s", maskAPIKeyInString(err.Error(), apiKey)))
	}

	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s.
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("building request: %w", maskErr(err))
		}
		req.Header.Set("User-Agent", "wellspring-cli/0.1")

		resp, err := a.client.Do(req)
		if err != nil {
			// Mask API key in error messages to avoid leaking credentials.
			lastErr = fmt.Errorf("API request failed: %w", maskErr(err))
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("Alpha Vantage returned status %d: %s", resp.StatusCode, maskAPIKeyInString(truncateBody(string(body)), apiKey))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Alpha Vantage returned status %d: %s", resp.StatusCode, maskAPIKeyInString(truncateBody(string(body)), apiKey))
		}

		if readErr != nil {
			lastErr = fmt.Errorf("reading response: %w", readErr)
			continue
		}

		return body, nil
	}

	return nil, lastErr
}

// checkAVError inspects a parsed Alpha Vantage response for API-level errors.
// AV returns HTTP 200 for all responses, including rate limits and errors,
// signaled via "Note" or "Information" top-level keys.
func checkAVError(result map[string]any) error {
	if note, ok := result["Note"]; ok {
		return adapter.NewRateLimitError(fmt.Sprintf("rate limited by Alpha Vantage: %v\n\nHint: free tier allows 5 requests/minute", note))
	}
	if info, ok := result["Information"]; ok {
		return fmt.Errorf("Alpha Vantage: %v", info)
	}
	return nil
}

// maskAPIKey replaces API key values in error messages to prevent credential leakage.
func maskAPIKey(err error) error {
	msg := err.Error()
	// Mask apikey query parameter values in URLs.
	if idx := strings.Index(msg, "apikey="); idx >= 0 {
		end := strings.IndexAny(msg[idx+7:], "&\" ")
		if end == -1 {
			msg = msg[:idx+7] + "***"
		} else {
			msg = msg[:idx+7] + "***" + msg[idx+7+end:]
		}
	}
	return fmt.Errorf("%s", msg)
}

// maskAPIKeyInString replaces literal occurrences of the API key in a string.
func maskAPIKeyInString(s, apiKey string) string {
	if apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, apiKey, "***")
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
