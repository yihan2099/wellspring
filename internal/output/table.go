package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/wellspring-cli/wellspring/internal/adapter"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	cellStyle   = lipgloss.NewStyle()
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// RenderTable renders DataPoints as a human-readable table.
func RenderTable(w io.Writer, points []adapter.DataPoint, noColor bool) {
	if len(points) == 0 {
		fmt.Fprintln(w, "No results.")
		return
	}

	if noColor {
		headerStyle = lipgloss.NewStyle().Bold(true)
		cellStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	// Determine columns based on what data is available.
	hasTitle := false
	hasValue := false
	hasURL := false
	for _, p := range points {
		if p.Title != "" {
			hasTitle = true
		}
		if p.Value != nil {
			hasValue = true
		}
		if p.URL != "" {
			hasURL = true
		}
	}

	// Build column headers.
	cols := []string{"#"}
	if hasTitle {
		cols = append(cols, "Title")
	}
	if hasValue {
		cols = append(cols, "Value")
	}
	cols = append(cols, "Time")
	if hasURL {
		cols = append(cols, "URL")
	}

	// Calculate column widths.
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}

	rows := make([][]string, len(points))
	for i, p := range points {
		row := []string{fmt.Sprintf("%d", i+1)}
		ci := 1

		if hasTitle {
			title := truncate(p.Title, 60)
			row = append(row, title)
			if len(title) > widths[ci] {
				widths[ci] = len(title)
			}
			ci++
		}
		if hasValue {
			val := fmt.Sprintf("%v", p.Value)
			row = append(row, val)
			if len(val) > widths[ci] {
				widths[ci] = len(val)
			}
			ci++
		}
		timeStr := formatTime(p.Time)
		row = append(row, timeStr)
		if len(timeStr) > widths[ci] {
			widths[ci] = len(timeStr)
		}
		ci++

		if hasURL {
			url := truncate(p.URL, 50)
			row = append(row, url)
			if len(url) > widths[ci] {
				widths[ci] = len(url)
			}
		}
		rows[i] = row
	}

	// Width of "#" column.
	numW := len(fmt.Sprintf("%d", len(points)))
	if numW < widths[0] {
		numW = widths[0]
	}
	widths[0] = numW

	// Print header.
	header := ""
	for i, c := range cols {
		header += headerStyle.Render(padRight(c, widths[i])) + "  "
	}
	fmt.Fprintln(w, header)

	// Print separator.
	sep := ""
	for i := range cols {
		sep += strings.Repeat("─", widths[i]) + "  "
	}
	fmt.Fprintln(w, dimStyle.Render(sep))

	// Print rows.
	for _, row := range rows {
		line := ""
		for i, cell := range row {
			if i == 0 {
				line += dimStyle.Render(padRight(cell, widths[i])) + "  "
			} else {
				line += cellStyle.Render(padRight(cell, widths[i])) + "  "
			}
		}
		fmt.Fprintln(w, line)
	}

	// Print meta info from first point.
	if len(points) > 0 {
		src := points[0].Source
		cat := points[0].Category
		fmt.Fprintln(w)
		fmt.Fprintln(w, dimStyle.Render(fmt.Sprintf("  %s/%s · %d results", cat, src, len(points))))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return t.Format("2006-01-02")
	}
}
