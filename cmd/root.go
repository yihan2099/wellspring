package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/adapter/coded"
	"github.com/wellspring-cli/wellspring/internal/adapter/declarative"
	"github.com/wellspring-cli/wellspring/internal/config"
	"github.com/wellspring-cli/wellspring/internal/output"
	"github.com/wellspring-cli/wellspring/internal/ratelimit"
	"github.com/wellspring-cli/wellspring/internal/registry"
)

var (
	// Version is set at build time.
	Version = "0.1.0-dev"

	// Global flags.
	flagJSON    bool
	flagPlain   bool
	flagQuiet   bool
	flagNoColor bool
	flagLimit   int
	flagCache   string
	flagOffline bool
	flagConfig  string
	flagDebug   bool

	// Global state.
	reg     *registry.Registry
	limiter *ratelimit.Limiter
	cache   *ratelimit.Cache
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "wsp",
	Short: "Wellspring — one CLI for any public data source",
	Long: `Wellspring (wsp) is a unified CLI that turns public-apis into callable tools.

One binary. One interface. Any public data source.

Quick start:
  wsp news top                          Hacker News top stories
  wsp news top --source=reddit          Reddit hot posts
  wsp weather forecast --lat=40.7 --lon=-74.0  Weather forecast
  wsp crypto prices                     Cryptocurrency prices
  wsp finance quote --symbol=AAPL       Stock quote (needs API key)
  wsp government population --country=US  Population data
  wsp sources                           List all known APIs
  wsp sources --supported               List callable sources
  wsp serve                             Start MCP server for agents

Configuration:
  Config file: ~/.config/wellspring/config.toml
  Cache dir:   ~/.cache/wellspring/
  Custom sources: ~/.config/wellspring/sources/*.yaml

Environment variables:
  WSP_ALPHA_VANTAGE_KEY   Alpha Vantage API key
  WSP_CONFIG_DIR          Override config directory
  WSP_CACHE_DIR           Override cache directory
  NO_COLOR                Disable color output`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize global state after flags are parsed so --cache, --offline,
		// and --debug take effect during initialization.
		initOnce.Do(initGlobals)
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand given, show quick-start guide.
		return cmd.Help()
	},
}

func init() {
	// Global flags.
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Structured JSON output")
	rootCmd.PersistentFlags().BoolVar(&flagPlain, "plain", false, "Tab-separated plain text output")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress all non-data output")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().IntVarP(&flagLimit, "limit", "n", 10, "Maximum number of results")
	rootCmd.PersistentFlags().StringVar(&flagCache, "cache", "", "Cache duration override (e.g., 5m, 1h)")
	rootCmd.PersistentFlags().BoolVar(&flagOffline, "offline", false, "Skip registry sync, use built-in/cached sources only")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&flagDebug, "debug", false, "Show request/response details on stderr")

	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("wsp version {{.Version}}\n")

	// Register all subcommands.
	rootCmd.AddCommand(newsCmd)
	rootCmd.AddCommand(weatherCmd)
	rootCmd.AddCommand(cryptoCmd)
	rootCmd.AddCommand(financeCmd)
	rootCmd.AddCommand(governmentCmd)
	rootCmd.AddCommand(sourcesCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(serveCmd)
}

var initOnce sync.Once

// Execute runs the root command.
func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		if flagJSON {
			output.RenderJSONError(os.Stdout, "", err)
		} else if !flagQuiet {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return err
}

func initGlobals() {
	// Load config.
	cfg = config.DefaultConfig()

	// Check NO_COLOR env var.
	if os.Getenv("NO_COLOR") != "" {
		flagNoColor = true
	}

	// Initialize rate limiter.
	limiter = ratelimit.NewLimiter()

	// Initialize cache.
	cacheTTL := cfg.General.CacheTTL
	if flagCache != "" {
		if d, err := time.ParseDuration(flagCache); err == nil {
			cacheTTL = d
		}
	}
	cache = ratelimit.NewCache(cacheTTL)

	// Initialize registry.
	reg = registry.NewRegistry()

	// Load built-in declarative adapters.
	loadBuiltInSources()

	// Register coded adapters.
	reg.Register(coded.NewRedditAdapter())
	reg.Register(coded.NewAlphaVantageAdapter(cfg.GetAPIKey("ALPHA_VANTAGE")))

	// Load catalog.
	reg.LoadCatalog()

	// Load user-defined sources (highest priority — may override built-in).
	reg.LoadUserSources()

	// Background sync if not offline.
	if !flagOffline {
		status := registry.LoadSyncStatus()
		if status.NeedsBackgroundRefresh() {
			registry.BackgroundSync(reg, flagDebug)
		}
	}
}

// loadBuiltInSources loads the declarative YAML adapters compiled into the binary.
func loadBuiltInSources() {
	sources := map[string][]byte{
		"hackernews": hackernewsYAML,
		"openmeteo":  openmeteoYAML,
		"coingecko":  coingeckoYAML,
		"worldbank":  worldbankYAML,
	}

	for name, data := range sources {
		a, err := declarative.LoadFromBytes(data)
		if err != nil {
			if flagDebug {
				fmt.Fprintf(os.Stderr, "[debug] failed to load built-in source %s: %v\n", name, err)
			}
			continue
		}
		reg.Register(a)
	}
}

// getOutputFormat determines the output format based on flags and TTY detection.
func getOutputFormat() output.Format {
	if flagJSON {
		return output.FormatJSON
	}
	if flagPlain {
		return output.FormatPlain
	}
	return output.AutoDetectFormat()
}

// exitCode returns the appropriate exit code for an error.
// Uses sentinel error types (errors.Is) for reliable classification,
// with string-matching fallback for errors from external packages.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, adapter.ErrRateLimit):
		return 3
	case errors.Is(err, adapter.ErrAuthRequired):
		return 2
	case errors.Is(err, adapter.ErrInvalidInput):
		return 4
	default:
		// Fallback: string matching for errors not yet migrated to sentinels.
		errStr := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errStr, "rate limit"):
			return 3
		case strings.Contains(errStr, "requires an api key"), strings.Contains(errStr, "auth"):
			return 2
		case strings.Contains(errStr, "invalid"), strings.Contains(errStr, "unknown action"):
			return 4
		default:
			return 1
		}
	}
}
