package coded

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
)

// RedditAdapter implements adapter.Adapter for the Reddit JSON API.
// Reddit provides JSON endpoints by appending .json to any URL.
type RedditAdapter struct {
	client *http.Client
}

// NewRedditAdapter creates a new Reddit adapter.
func NewRedditAdapter() *RedditAdapter {
	return &RedditAdapter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (a *RedditAdapter) Name() string        { return "reddit" }
func (a *RedditAdapter) Category() string     { return "news" }
func (a *RedditAdapter) Description() string  { return "Reddit — subreddit posts, hot/top/new" }
func (a *RedditAdapter) RequiresAuth() bool   { return false }
func (a *RedditAdapter) Endpoints() []string  { return []string{"hot", "top", "new", "rising"} }

func (a *RedditAdapter) RateLimit() adapter.RateLimitConfig {
	return adapter.RateLimitConfig{
		Requests: 30,
		Per:      time.Minute,
	}
}

func (a *RedditAdapter) ToolParams(endpoint string) []adapter.ToolParam {
	params := []adapter.ToolParam{
		{Name: "limit", Description: "Maximum number of results", Default: "10"},
		{Name: "subreddit", Description: "Subreddit name", Default: "technology"},
	}
	if endpoint == "top" {
		params = append(params, adapter.ToolParam{
			Name: "time", Description: "Time filter (hour, day, week, month, year, all)", Default: "day",
		})
	}
	return params
}

// redditListing represents the Reddit API listing response.
type redditListing struct {
	Data struct {
		Children []struct {
			Data redditPost `json:"data"`
		} `json:"children"`
		After string `json:"after"`
	} `json:"data"`
}

type redditPost struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Permalink   string  `json:"permalink"`
	Subreddit   string  `json:"subreddit"`
	Author      string  `json:"author"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	CreatedUTC  float64 `json:"created_utc"`
	Selftext    string  `json:"selftext"`
	Domain      string  `json:"domain"`
	IsVideo     bool    `json:"is_video"`
	Thumbnail   string  `json:"thumbnail"`
	Ups         int     `json:"ups"`
	Downs       int     `json:"downs"`
}

func (a *RedditAdapter) Fetch(ctx context.Context, params map[string]string) ([]adapter.DataPoint, error) {
	if params == nil {
		params = make(map[string]string)
	}
	action := params["action"]
	if action == "" {
		action = "hot"
	}

	// Validate action against known endpoints.
	validActions := map[string]bool{"hot": true, "top": true, "new": true, "rising": true}
	if !validActions[action] {
		return nil, adapter.NewInvalidInputError(
			fmt.Sprintf("unknown action %q for reddit (available: hot, top, new, rising)", action))
	}

	subreddit := params["subreddit"]
	if subreddit == "" {
		subreddit = "technology"
	}

	limit := 10
	if l, ok := params["limit"]; ok {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	// Clamp limit to valid range.
	if limit < 1 {
		limit = 1
	} else if limit > 100 {
		limit = 100
	}

	// Build URL with path-escaped subreddit name to handle special characters.
	reqURL := fmt.Sprintf("https://www.reddit.com/r/%s/%s.json?limit=%d&raw_json=1",
		url.PathEscape(subreddit), url.PathEscape(action), limit)

	// Add time filter for "top".
	if action == "top" {
		t := params["time"]
		if t == "" {
			t = "day"
		}
		reqURL += "&t=" + t
	}

	const maxRetries = 3
	var body []byte
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

		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("building request: %w", err)
		}

		// Reddit requires a proper User-Agent to avoid rate limiting.
		req.Header.Set("User-Agent", "wellspring-cli/0.1 (github.com/wellspring-cli/wellspring)")

		resp, err := a.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("fetching r/%s/%s: %w", subreddit, action, err)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("Reddit API returned status %d: %s", resp.StatusCode, truncateBody(string(respBody)))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Reddit API returned status %d: %s", resp.StatusCode, truncateBody(string(respBody)))
		}

		if readErr != nil {
			lastErr = fmt.Errorf("reading response: %w", readErr)
			continue
		}

		body = respBody
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, lastErr
	}

	var listing redditListing
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("parsing Reddit response: %w", err)
	}

	points := make([]adapter.DataPoint, 0, len(listing.Data.Children))
	for _, child := range listing.Data.Children {
		post := child.Data
		dp := adapter.DataPoint{
			Source:   "reddit",
			Category: "news",
			Title:   post.Title,
			URL:     "https://www.reddit.com" + post.Permalink,
			Value:   post.Score,
			Time:    time.Unix(int64(post.CreatedUTC), 0),
			Meta: map[string]any{
				"author":    post.Author,
				"subreddit": post.Subreddit,
				"comments":  post.NumComments,
				"domain":    post.Domain,
			},
		}
		points = append(points, dp)
	}

	return points, nil
}

func truncateBody(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
