package coded

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	action := params["action"]
	if action == "" {
		action = "hot"
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

	// Build URL.
	url := fmt.Sprintf("https://www.reddit.com/r/%s/%s.json?limit=%d&raw_json=1",
		subreddit, action, limit)

	// Add time filter for "top".
	if action == "top" {
		t := params["time"]
		if t == "" {
			t = "day"
		}
		url += "&t=" + t
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	// Reddit requires a proper User-Agent to avoid rate limiting.
	req.Header.Set("User-Agent", "wellspring-cli/0.1 (github.com/wellspring-cli/wellspring)")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching r/%s/%s: %w", subreddit, action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by Reddit — try again in a moment")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Reddit API returned status %d: %s", resp.StatusCode, truncateBody(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
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
