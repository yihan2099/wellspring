package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/wellspring-cli/wellspring/internal/adapter"
)

// JSONOutput wraps results in a consistent schema.
type JSONOutput struct {
	OK      bool              `json:"ok"`
	Source  string            `json:"source"`
	Count   int               `json:"count"`
	Results []adapter.DataPoint `json:"results"`
	Error   string            `json:"error,omitempty"`
}

// RenderJSON renders DataPoints as structured JSON.
func RenderJSON(w io.Writer, points []adapter.DataPoint) {
	source := ""
	if len(points) > 0 {
		source = points[0].Source
	}

	out := JSONOutput{
		OK:      true,
		Source:  source,
		Count:   len(points),
		Results: points,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// RenderJSONError renders an error as structured JSON.
func RenderJSONError(w io.Writer, source string, err error) {
	out := JSONOutput{
		OK:     false,
		Source: source,
		Count:  0,
		Error:  err.Error(),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

// RenderJSONRaw renders any value as JSON (for generic output).
func RenderJSONRaw(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(w, `{"ok": false, "error": %q}`, err.Error())
	}
}
