package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/output"
	"github.com/wellspring-cli/wellspring/internal/ratelimit"
)

var (
	financeSymbol string
	financeQuery  string
)

var financeCmd = &cobra.Command{
	Use:   "finance",
	Short: "Stock quotes and financial data",
	Long: `Fetch financial data from Alpha Vantage (requires free API key).

Get a free API key: https://www.alphavantage.co/support/#api-key
Set it via: export WSP_ALPHA_VANTAGE_KEY=your_key

Examples:
  wsp finance quote --symbol=AAPL        Apple stock quote
  wsp finance daily --symbol=MSFT        Microsoft daily prices
  wsp finance search --query=Tesla       Search for symbols
  wsp finance quote --symbol=GOOGL --json  JSON output`,
}

var financeQuoteCmd = &cobra.Command{
	Use:   "quote",
	Short: "Real-time stock quote",
	Long: `Get a real-time stock quote for a given symbol.

Examples:
  wsp finance quote --symbol=AAPL        Apple stock quote
  wsp finance quote --symbol=MSFT        Microsoft stock quote
  wsp finance quote --symbol=GOOGL --json  JSON output`,
	RunE: runFinanceAction("quote"),
}

var financeDailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "Daily stock prices",
	Long: `Get daily historical stock prices for a given symbol.

Examples:
  wsp finance daily --symbol=AAPL         Apple daily prices
  wsp finance daily --symbol=MSFT --limit=30  Last 30 days`,
	RunE: runFinanceAction("daily"),
}

var financeSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search for stock symbols",
	Long: `Search for stock symbols by company name.

Examples:
  wsp finance search --query=Apple       Search for Apple
  wsp finance search --query=Tesla       Search for Tesla`,
	RunE: runFinanceAction("search"),
}

func init() {
	financeCmd.PersistentFlags().StringVar(&financeSymbol, "symbol", "", "Stock ticker symbol (e.g., AAPL, MSFT)")
	financeCmd.PersistentFlags().StringVar(&financeQuery, "query", "", "Search query for symbol lookup")

	financeCmd.AddCommand(financeQuoteCmd)
	financeCmd.AddCommand(financeDailyCmd)
	financeCmd.AddCommand(financeSearchCmd)
}

func runFinanceAction(action string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		a, ok := reg.Get("alphavantage")
		if !ok {
			return fmt.Errorf("Alpha Vantage adapter not found — this is a bug, please report it")
		}

		params := map[string]string{
			"action": action,
			"limit":  fmt.Sprintf("%d", flagLimit),
			"symbol": financeSymbol,
			"query":  financeQuery,
		}

		// Pass API key from config if available.
		if key := cfg.GetAPIKey("ALPHA_VANTAGE"); key != "" {
			params["api_key"] = key
		}

		// Check rate limit.
		if ok, wait := limiter.Allow(a.Name(), a.RateLimit()); !ok {
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
			fmt.Fprintf(os.Stderr, "[debug] fetching from alphavantage (action=%s, symbol=%s)\n", action, financeSymbol)
		}

		ctx := cmd.Context()
		points, err := a.Fetch(ctx, params)
		if err != nil {
			return err
		}

		cache.Set(a.Name(), params, points)
		output.Render(os.Stdout, points, getOutputFormat(), flagNoColor)
		return nil
	}
}
