package declarative

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"gopkg.in/yaml.v3"
)

// SourceDefinition represents a YAML source definition file.
type SourceDefinition struct {
	Name        string                      `yaml:"name"`
	CategoryStr string                      `yaml:"category"`
	DescStr     string                      `yaml:"description"`
	Auth        string                      `yaml:"auth"`
	BaseURL     string                      `yaml:"base_url"`
	Endpoints   map[string]EndpointDef      `yaml:"endpoints"`
	Mapping     MappingDef                  `yaml:"mapping"`
	RateLimitDef RateLimitDef               `yaml:"rate_limit"`
}

// EndpointDef defines an API endpoint.
type EndpointDef struct {
	Path       string            `yaml:"path"`
	Method     string            `yaml:"method"`
	Params     map[string]string `yaml:"params"`
	Headers    map[string]string `yaml:"headers"`
	Pagination string            `yaml:"pagination"`
}

// MappingDef defines how to map API response fields to DataPoint fields.
type MappingDef struct {
	Title    string            `yaml:"title"`
	URL      string            `yaml:"url"`
	Time     string            `yaml:"time"`
	Value    string            `yaml:"value"`
	Meta     map[string]string `yaml:"meta"`
	// For APIs that return arrays of IDs that need individual fetching.
	ItemEndpoint string        `yaml:"item_endpoint"`
}

// RateLimitDef defines rate limiting in YAML.
type RateLimitDef struct {
	Requests int    `yaml:"requests"`
	Per      string `yaml:"per"`
}

// DeclarativeAdapter implements adapter.Adapter using a YAML definition.
type DeclarativeAdapter struct {
	def    SourceDefinition
	client *http.Client
}

// LoadFromFile loads a declarative adapter from a YAML file.
func LoadFromFile(path string) (*DeclarativeAdapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading source definition: %w", err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes loads a declarative adapter from YAML bytes.
func LoadFromBytes(data []byte) (*DeclarativeAdapter, error) {
	var def SourceDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing source definition: %w", err)
	}

	if def.Name == "" {
		return nil, fmt.Errorf("source definition missing 'name'")
	}
	if def.BaseURL == "" {
		return nil, fmt.Errorf("source definition missing 'base_url'")
	}

	return &DeclarativeAdapter{
		def: def,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// LoadAllFromDir loads all YAML source definitions from a directory.
func LoadAllFromDir(dir string) ([]*DeclarativeAdapter, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var adapters []*DeclarativeAdapter
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		a, err := LoadFromFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			// Log warning but continue loading other sources.
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}
		adapters = append(adapters, a)
	}
	return adapters, nil
}

func (a *DeclarativeAdapter) Name() string     { return a.def.Name }
func (a *DeclarativeAdapter) Category() string  { return a.def.CategoryStr }
func (a *DeclarativeAdapter) Description() string {
	if a.def.DescStr != "" {
		return a.def.DescStr
	}
	return fmt.Sprintf("%s data from %s", a.def.CategoryStr, a.def.Name)
}

func (a *DeclarativeAdapter) RequiresAuth() bool {
	return a.def.Auth != "" && a.def.Auth != "none"
}

func (a *DeclarativeAdapter) Endpoints() []string {
	endpoints := make([]string, 0, len(a.def.Endpoints))
	for name := range a.def.Endpoints {
		endpoints = append(endpoints, name)
	}
	sort.Strings(endpoints)
	return endpoints
}

// EndpointParams returns the parameter names declared for a given endpoint.
// This allows MCP tool registration to expose source-specific parameters
// without hardcoding them in a switch statement.
func (a *DeclarativeAdapter) EndpointParams(endpoint string) map[string]string {
	if ep, ok := a.def.Endpoints[endpoint]; ok {
		return ep.Params
	}
	return nil
}

// EndpointPathParams returns parameter names extracted from path templates
// (e.g., /country/{country}/indicator/{indicator} yields ["country", "indicator"]).
func (a *DeclarativeAdapter) EndpointPathParams(endpoint string) []string {
	ep, ok := a.def.Endpoints[endpoint]
	if !ok {
		return nil
	}
	var params []string
	path := ep.Path
	for {
		start := strings.Index(path, "{")
		if start == -1 {
			break
		}
		end := strings.Index(path[start:], "}")
		if end == -1 {
			break
		}
		param := path[start+1 : start+end]
		params = append(params, param)
		path = path[start+end+1:]
	}
	return params
}

// ToolParams returns MCP tool parameter definitions derived from the YAML
// endpoint definition (query params + path template params).
func (a *DeclarativeAdapter) ToolParams(endpoint string) []adapter.ToolParam {
	// Check if the YAML endpoint already declares a "limit" param to avoid
	// registering a duplicate with a conflicting default.
	hasLimit := false
	if ep, ok := a.def.Endpoints[endpoint]; ok {
		if _, exists := ep.Params["limit"]; exists {
			hasLimit = true
		}
	}

	var params []adapter.ToolParam
	if !hasLimit {
		params = append(params, adapter.ToolParam{
			Name: "limit", Description: "Maximum number of results", Default: "10",
		})
	}

	// Add YAML-declared query params with their defaults.
	if ep, ok := a.def.Endpoints[endpoint]; ok {
		for k, v := range ep.Params {
			params = append(params, adapter.ToolParam{
				Name: k, Description: k + " parameter", Default: v,
			})
		}
	}

	// Add path template params (e.g., {country}, {id}).
	for _, p := range a.EndpointPathParams(endpoint) {
		if p == "id" {
			continue // internal resolution param
		}
		params = append(params, adapter.ToolParam{
			Name: p, Description: p + " parameter",
		})
	}

	return params
}

func (a *DeclarativeAdapter) RateLimit() adapter.RateLimitConfig {
	dur := time.Minute // default
	if a.def.RateLimitDef.Per != "" {
		if parsed, err := time.ParseDuration(a.def.RateLimitDef.Per); err == nil {
			dur = parsed
		}
	}
	reqs := a.def.RateLimitDef.Requests
	if reqs == 0 {
		reqs = 30 // default
	}
	return adapter.RateLimitConfig{
		Requests: reqs,
		Per:      dur,
	}
}

// Fetch retrieves data from the API using the declarative definition.
func (a *DeclarativeAdapter) Fetch(ctx context.Context, params map[string]string) ([]adapter.DataPoint, error) {
	action := params["action"]
	if action == "" {
		// Default to first endpoint.
		for name := range a.def.Endpoints {
			action = name
			break
		}
	}

	ep, ok := a.def.Endpoints[action]
	if !ok {
		available := make([]string, 0, len(a.def.Endpoints))
		for name := range a.def.Endpoints {
			available = append(available, name)
		}
		return nil, fmt.Errorf("unknown action %q for %s (available: %s)", action, a.def.Name, strings.Join(available, ", "))
	}

	limit := 10
	if l, ok := params["limit"]; ok {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}

	// Build URL with path parameters, escaping values to prevent
	// path traversal or injection via special characters (/, ?, #, etc.).
	path := ep.Path
	for k, v := range params {
		path = strings.ReplaceAll(path, "{"+k+"}", url.PathEscape(v))
	}
	apiURL := a.def.BaseURL + path

	// Add query parameters (URL-encoded to prevent injection).
	if len(ep.Params) > 0 {
		sep := "?"
		if strings.Contains(apiURL, "?") {
			sep = "&"
		}
		for k, v := range ep.Params {
			// Allow param override from user params.
			if userVal, ok := params[k]; ok {
				v = userVal
			}
			apiURL += sep + url.QueryEscape(k) + "=" + url.QueryEscape(v)
			sep = "&"
		}
	}

	method := ep.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	// Add headers.
	req.Header.Set("User-Agent", "wellspring-cli/0.1")
	for k, v := range ep.Headers {
		req.Header.Set(k, v)
	}

	// Handle auth.
	if a.def.Auth == "apiKey" {
		if key, ok := params["api_key"]; ok && key != "" {
			req.Header.Set("X-API-Key", key)
		}
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return a.parseResponse(ctx, body, limit, params)
}

// parseResponse parses the JSON response and maps it to DataPoints.
func (a *DeclarativeAdapter) parseResponse(ctx context.Context, body []byte, limit int, params map[string]string) ([]adapter.DataPoint, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	// If the response is an array of IDs and we have an item endpoint, fetch each item.
	if a.def.Mapping.ItemEndpoint != "" {
		return a.fetchItems(ctx, raw, limit, params)
	}

	// If response is an array, map each element.
	if arr, ok := raw.([]any); ok {
		return a.mapArray(arr, limit)
	}

	// If response is an object, try to extract array from a known field.
	if obj, ok := raw.(map[string]any); ok {
		// Try common array fields.
		for _, key := range []string{"data", "results", "items", "list"} {
			if arr, ok := obj[key].([]any); ok {
				return a.mapArray(arr, limit)
			}
		}
		// Single object result.
		dp := a.mapObject(obj)
		return []adapter.DataPoint{dp}, nil
	}

	return nil, fmt.Errorf("unexpected response format")
}

// fetchItems handles APIs that return an array of IDs, then fetches each item.
// A hard cap of 50 items prevents runaway HTTP requests when a large limit
// or no limit is specified (e.g., HN returns 500 IDs per list endpoint).
const maxResolveItems = 50

func (a *DeclarativeAdapter) fetchItems(ctx context.Context, raw any, limit int, params map[string]string) ([]adapter.DataPoint, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array of IDs, got %T", raw)
	}

	if limit > 0 && len(arr) > limit {
		arr = arr[:limit]
	}
	// Hard cap to prevent excessive HTTP requests.
	if len(arr) > maxResolveItems {
		arr = arr[:maxResolveItems]
	}

	itemEp, ok := a.def.Endpoints[a.def.Mapping.ItemEndpoint]
	if !ok {
		return nil, fmt.Errorf("item endpoint %q not found", a.def.Mapping.ItemEndpoint)
	}

	points := make([]adapter.DataPoint, 0, len(arr))
	var failures int
	for _, idRaw := range arr {
		id := fmt.Sprintf("%v", idRaw)
		// Handle float64 IDs (JSON numbers are decoded as float64).
		// Validate that the float-to-int conversion is lossless to catch
		// IDs that exceed int64 precision (~2^53 for float64).
		if f, ok := idRaw.(float64); ok {
			n := int64(f)
			if float64(n) != f {
				// Fractional or out-of-range ID — use the string representation
				// with the ".0" suffix stripped as a best-effort fallback.
				if strings.HasSuffix(id, ".0") {
					id = id[:len(id)-2]
				}
			} else {
				id = strconv.FormatInt(n, 10)
			}
		} else if strings.HasSuffix(id, ".0") {
			id = id[:len(id)-2]
		}

		path := strings.ReplaceAll(itemEp.Path, "{id}", url.PathEscape(id))
		itemURL := a.def.BaseURL + path

		req, err := http.NewRequestWithContext(ctx, "GET", itemURL, nil)
		if err != nil {
			failures++
			continue
		}
		req.Header.Set("User-Agent", "wellspring-cli/0.1")

		resp, err := a.client.Do(req)
		if err != nil {
			failures++
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			failures++
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			failures++
			continue
		}

		dp := a.mapObject(obj)
		points = append(points, dp)
	}

	if failures > 0 {
		fmt.Fprintf(os.Stderr, "warning: %s: %d/%d item fetches failed\n", a.def.Name, failures, len(arr))
	}

	return points, nil
}

// mapArray maps an array of objects to DataPoints.
func (a *DeclarativeAdapter) mapArray(arr []any, limit int) ([]adapter.DataPoint, error) {
	if limit > 0 && len(arr) > limit {
		arr = arr[:limit]
	}

	points := make([]adapter.DataPoint, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		dp := a.mapObject(obj)
		points = append(points, dp)
	}
	return points, nil
}

// mapObject maps a single JSON object to a DataPoint using the mapping definition.
func (a *DeclarativeAdapter) mapObject(obj map[string]any) adapter.DataPoint {
	dp := adapter.DataPoint{
		Source:   a.def.Name,
		Category: a.def.CategoryStr,
		Meta:    make(map[string]any),
	}

	if a.def.Mapping.Title != "" {
		dp.Title = getString(obj, a.def.Mapping.Title)
	}
	if a.def.Mapping.URL != "" {
		dp.URL = getString(obj, a.def.Mapping.URL)
	}
	if a.def.Mapping.Value != "" {
		dp.Value = getValue(obj, a.def.Mapping.Value)
	}
	if a.def.Mapping.Time != "" {
		dp.Time = getTime(obj, a.def.Mapping.Time)
	}

	for metaKey, path := range a.def.Mapping.Meta {
		if v := getValue(obj, path); v != nil {
			dp.Meta[metaKey] = v
		}
	}

	return dp
}

// getString extracts a string value from a JSON object using a dot-path like ".title".
func getString(obj map[string]any, path string) string {
	v := getValue(obj, path)
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// getValue extracts a value from a JSON object using a dot-path like ".field".
func getValue(obj map[string]any, path string) any {
	// Handle pipe transformations (e.g., ".time | unix").
	parts := strings.SplitN(path, " | ", 2)
	fieldPath := strings.TrimSpace(parts[0])
	transform := ""
	if len(parts) > 1 {
		transform = strings.TrimSpace(parts[1])
	}

	// Remove leading dot.
	fieldPath = strings.TrimPrefix(fieldPath, ".")

	// Navigate nested fields.
	fields := strings.Split(fieldPath, ".")
	var current any = obj
	for _, field := range fields {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[field]
	}

	if current == nil {
		return nil
	}

	// Apply transformation.
	switch transform {
	case "unix":
		if f, ok := current.(float64); ok {
			return time.Unix(int64(f), 0)
		}
	}

	return current
}

// getTime extracts a time value from a JSON object.
func getTime(obj map[string]any, path string) time.Time {
	v := getValue(obj, path)
	if v == nil {
		return time.Time{}
	}

	switch t := v.(type) {
	case time.Time:
		return t
	case float64:
		return time.Unix(int64(t), 0)
	case string:
		// Try common formats.
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02",
		} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
