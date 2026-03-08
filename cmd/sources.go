package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/output"
	"github.com/wellspring-cli/wellspring/internal/registry"
)

var (
	sourcesCategory  string
	sourcesAuth      string
	sourcesSupported bool
	sourcesCheck     bool
)

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List available data sources",
	Long: `List all known APIs from the public-apis catalog and show which have Wellspring adapters.

Examples:
  wsp sources                            List all known APIs
  wsp sources --supported                Only sources with adapters
  wsp sources --category=news            Filter by category
  wsp sources --auth=none                Filter by auth type
  wsp sources --check                    Health-check supported sources
  wsp sources --json                     JSON output`,
	RunE: runSources,
}

func init() {
	sourcesCmd.Flags().StringVar(&sourcesCategory, "category", "", "Filter by category")
	sourcesCmd.Flags().StringVar(&sourcesAuth, "auth", "", "Filter by auth type (none, apiKey)")
	sourcesCmd.Flags().BoolVar(&sourcesSupported, "supported", false, "Show only supported sources")
	sourcesCmd.Flags().BoolVar(&sourcesCheck, "check", false, "Health-check supported source endpoints")
}

func runSources(cmd *cobra.Command, args []string) error {
	filter := registry.SourceFilter{
		Category: sourcesCategory,
		Auth:     sourcesAuth,
	}

	if sourcesSupported {
		t := true
		filter.Supported = &t
	}

	sources := reg.Sources(filter)

	// Health check mode.
	if sourcesCheck {
		return runHealthCheck(sources)
	}

	// JSON output.
	if flagJSON {
		output.RenderJSONRaw(os.Stdout, map[string]any{
			"ok":      true,
			"count":   len(sources),
			"sources": sources,
		})
		return nil
	}

	// Table/text output.
	fmt.Print(registry.FormatSourcesOutput(sources))
	return nil
}

func runHealthCheck(sources []registry.SourceInfo) error {
	if !flagQuiet {
		fmt.Fprintln(os.Stderr, "Checking supported source endpoints...")
	}

	type checkResult struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Latency string `json:"latency,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	var results []checkResult
	_ = &http.Client{Timeout: 10 * time.Second} // reserved for future direct endpoint checks

	for _, s := range sources {
		if !s.Supported {
			continue
		}

		a, ok := reg.Get(s.Name)
		if !ok {
			continue
		}

		// Try a minimal fetch to check if the API is responsive.
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		_, err := a.Fetch(ctx, map[string]string{
			"action": "",
			"limit":  "1",
		})
		cancel()
		elapsed := time.Since(start)

		result := checkResult{
			Name:    s.Name,
			Latency: elapsed.Round(time.Millisecond).String(),
		}

		if err != nil {
			// Auth errors are not health failures.
			if exitCode(err) == 2 {
				result.Status = "ok (needs auth)"
			} else {
				result.Status = "error"
				result.Error = err.Error()
			}
		} else {
			result.Status = "ok"
		}

		results = append(results, result)

		if !flagQuiet && !flagJSON {
			marker := "✓"
			if result.Status == "error" {
				marker = "✗"
			} else if result.Status == "ok (needs auth)" {
				marker = "⚷"
			}
			fmt.Fprintf(os.Stderr, "  %s %-20s %s  %s\n", marker, result.Name, result.Latency, result.Status)
		}
	}

	if flagJSON {
		output.RenderJSONRaw(os.Stdout, map[string]any{
			"ok":      true,
			"checks":  results,
		})
	}

	return nil
}
