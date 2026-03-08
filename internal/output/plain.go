package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/wellspring-cli/wellspring/internal/adapter"
)

// RenderPlain renders DataPoints as tab-separated plain text.
func RenderPlain(w io.Writer, points []adapter.DataPoint) {
	if len(points) == 0 {
		return
	}

	for _, p := range points {
		fields := []string{}

		if p.Title != "" {
			fields = append(fields, p.Title)
		}
		if p.Value != nil {
			fields = append(fields, fmt.Sprintf("%v", p.Value))
		}
		if !p.Time.IsZero() {
			fields = append(fields, p.Time.Format("2006-01-02T15:04:05Z"))
		}
		if p.URL != "" {
			fields = append(fields, p.URL)
		}

		fmt.Fprintln(w, strings.Join(fields, "\t"))
	}
}
