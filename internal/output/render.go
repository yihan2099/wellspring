package output

import (
	"io"
	"os"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"golang.org/x/term"
)

// Format represents an output format.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatPlain Format = "plain"
)

// AutoDetectFormat returns the appropriate format based on TTY detection.
// Non-TTY stdout defaults to JSON for piping.
func AutoDetectFormat() Format {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return FormatTable
	}
	return FormatJSON
}

// Render renders DataPoints using the specified format.
func Render(w io.Writer, points []adapter.DataPoint, format Format, noColor bool) {
	switch format {
	case FormatJSON:
		RenderJSON(w, points)
	case FormatPlain:
		RenderPlain(w, points)
	default:
		RenderTable(w, points, noColor)
	}
}
