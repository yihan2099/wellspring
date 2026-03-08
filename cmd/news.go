package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/output"
	"github.com/wellspring-cli/wellspring/internal/ratelimit"
)

var (
	newsSource    string
	newsSubreddit string
	newsTime      string
)

var newsCmd = &cobra.Command{
	Use:   "news",
	Short: "News from Hacker News, Reddit, and more",
	Long: `Fetch news from various sources.

Examples:
  wsp news top                           Hacker News top stories
  wsp news top --limit=20                Top 20 stories
  wsp news new                           Newest stories
  wsp news best                          Best stories
  wsp news top --source=reddit           Reddit hot posts
  wsp news top --source=reddit --subreddit=golang
  wsp news top --json                    JSON output for scripting`,
}

var newsTopCmd = &cobra.Command{
	Use:   "top",
	Short: "Top/hot stories",
	Long: `Fetch top stories from the specified news source.

Examples:
  wsp news top                           Top Hacker News stories
  wsp news top --source=reddit           Reddit hot posts
  wsp news top --source=reddit --subreddit=programming --time=week
  wsp news top --limit=5 --json          Top 5 as JSON`,
	RunE: runNewsAction("top"),
}

var newsNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Newest stories",
	Long: `Fetch newest stories from the specified news source.

Examples:
  wsp news new                           Newest Hacker News stories
  wsp news new --source=reddit           Reddit new posts`,
	RunE: runNewsAction("new"),
}

var newsBestCmd = &cobra.Command{
	Use:   "best",
	Short: "Best stories",
	Long: `Fetch best/highest-rated stories.

Examples:
  wsp news best                          Best Hacker News stories
  wsp news best --limit=20              Top 20 best stories`,
	RunE: runNewsAction("best"),
}

func init() {
	newsCmd.PersistentFlags().StringVar(&newsSource, "source", "hackernews", "News source (hackernews, reddit)")
	newsCmd.PersistentFlags().StringVar(&newsSubreddit, "subreddit", "technology", "Subreddit for Reddit source")
	newsCmd.PersistentFlags().StringVar(&newsTime, "time", "day", "Time filter for Reddit top (hour, day, week, month, year, all)")

	newsCmd.AddCommand(newsTopCmd)
	newsCmd.AddCommand(newsNewCmd)
	newsCmd.AddCommand(newsBestCmd)
}

func runNewsAction(action string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		a, ok := reg.Get(newsSource)
		if !ok {
			return fmt.Errorf("unknown news source %q\n\nAvailable sources: hackernews, reddit\nRun 'wsp sources --category=news' to see all news sources", newsSource)
		}

		params := map[string]string{
			"action":    action,
			"limit":     fmt.Sprintf("%d", flagLimit),
			"subreddit": newsSubreddit,
			"time":      newsTime,
		}

		// Check rate limit.
		if ok, wait := limiter.Allow(a.Name(), a.RateLimit()); !ok {
			os.Exit(3)
			return fmt.Errorf("%s", ratelimit.FormatRateLimitError(a.Name(), wait))
		}

		// Check cache.
		if points, ok := cache.Get(a.Name(), params); ok {
			if flagDebug {
				fmt.Fprintln(os.Stderr, "[debug] serving from cache")
			}
			output.Render(os.Stdout, points, getOutputFormat(), flagNoColor)
			return nil
		}

		if flagDebug {
			fmt.Fprintf(os.Stderr, "[debug] fetching from %s (action=%s)\n", a.Name(), action)
		}

		ctx := context.Background()
		points, err := a.Fetch(ctx, params)
		if err != nil {
			os.Exit(exitCode(err))
			return err
		}

		// Store in cache.
		cache.Set(a.Name(), params, points)

		output.Render(os.Stdout, points, getOutputFormat(), flagNoColor)
		return nil
	}
}
