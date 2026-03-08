package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wellspring-cli/wellspring/internal/registry"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Force refresh of the source catalog and adapters",
	Long: `Force an immediate refresh of the API catalog and adapter registry.

This downloads the latest catalog from the remote registry, bypassing
the normal background sync schedule.

Examples:
  wsp update                             Refresh everything
  wsp update --debug                     Show sync details`,
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if !flagQuiet {
		fmt.Fprintln(os.Stderr, "Syncing registry...")
	}

	err := registry.ForceSync(reg, flagDebug)
	if err != nil {
		// Not fatal — we can still use built-in sources.
		if !flagQuiet {
			fmt.Fprintf(os.Stderr, "Warning: registry sync failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "Using built-in sources.")
		}
		// This is expected if the remote registry doesn't exist yet.
		return nil
	}

	if !flagQuiet {
		fmt.Fprintln(os.Stderr, "Registry updated successfully.")
	}

	// Clear response cache on update.
	cache.Clear()
	if !flagQuiet {
		fmt.Fprintln(os.Stderr, "Response cache cleared.")
	}

	return nil
}
