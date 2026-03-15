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
	cryptoCoin string
)

var cryptoCmd = &cobra.Command{
	Use:   "crypto",
	Short: "Cryptocurrency prices and market data",
	Long: `Fetch cryptocurrency data from CoinGecko (no API key required).

Examples:
  wsp crypto prices                      Top cryptocurrencies by market cap
  wsp crypto prices --limit=20           Top 20 cryptocurrencies
  wsp crypto trending                    Trending coins on CoinGecko
  wsp crypto prices --json               JSON output for scripting`,
}

var cryptoPricesCmd = &cobra.Command{
	Use:   "prices",
	Short: "Current cryptocurrency prices",
	Long: `Fetch current prices for top cryptocurrencies.

Examples:
  wsp crypto prices                      Top 10 by market cap
  wsp crypto prices --limit=25           Top 25
  wsp crypto prices --json               JSON output`,
	RunE: runCryptoAction("prices"),
}

var cryptoTrendingCmd = &cobra.Command{
	Use:   "trending",
	Short: "Trending cryptocurrencies",
	Long: `Fetch trending coins on CoinGecko.

Examples:
  wsp crypto trending                    Currently trending coins
  wsp crypto trending --json             JSON output`,
	RunE: runCryptoAction("trending"),
}

func init() {
	cryptoCmd.PersistentFlags().StringVar(&cryptoCoin, "coin", "", "Specific coin ID (e.g., bitcoin, ethereum)")

	cryptoCmd.AddCommand(cryptoPricesCmd)
	cryptoCmd.AddCommand(cryptoTrendingCmd)
}

func runCryptoAction(action string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		rc := getRunContext()

		a, ok := rc.Reg.Get("coingecko")
		if !ok {
			return fmt.Errorf("CoinGecko adapter not found — this is a bug, please report it")
		}

		params := map[string]string{
			"action":   action,
			"limit":    fmt.Sprintf("%d", rc.Limit),
			"per_page": fmt.Sprintf("%d", rc.Limit),
		}

		if cryptoCoin != "" {
			params["id"] = cryptoCoin
		}

		// Check rate limit.
		if ok, wait := rc.Limiter.Allow(a.Name(), a.RateLimit()); !ok {
			return fmt.Errorf("%s", ratelimit.FormatRateLimitError(a.Name(), wait))
		}

		// Check cache.
		if points, ok := rc.Cache.Get(a.Name(), params); ok {
			if rc.Debug {
				fmt.Fprintln(os.Stderr, "[debug] serving from cache")
			}
			output.Render(os.Stdout, points, getOutputFormat(), rc.NoColor)
			return nil
		}

		if rc.Debug {
			fmt.Fprintf(os.Stderr, "[debug] fetching from coingecko (action=%s)\n", action)
		}

		ctx := context.Background()
		points, err := a.Fetch(ctx, params)
		if err != nil {
			return err
		}

		if err := rc.Cache.Set(a.Name(), params, points); err != nil && rc.Debug {
			fmt.Fprintf(os.Stderr, "[debug] cache write failed: %v\n", err)
		}
		output.Render(os.Stdout, points, getOutputFormat(), rc.NoColor)
		return nil
	}
}
