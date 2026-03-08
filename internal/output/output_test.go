package output_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wellspring-cli/wellspring/internal/adapter"
	"github.com/wellspring-cli/wellspring/internal/output"
)

func samplePoints() []adapter.DataPoint {
	return []adapter.DataPoint{
		{
			Source:   "test",
			Category: "news",
			Title:   "First Item",
			Value:   42,
			Time:    time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
			URL:     "https://example.com/1",
			Meta:    map[string]any{"author": "alice"},
		},
		{
			Source:   "test",
			Category: "news",
			Title:   "Second Item",
			Value:   23,
			Time:    time.Date(2025, 6, 14, 8, 0, 0, 0, time.UTC),
			URL:     "https://example.com/2",
			Meta:    map[string]any{"author": "bob"},
		},
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	points := samplePoints()

	output.RenderJSON(&buf, points)

	var result output.JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if !result.OK {
		t.Error("expected ok=true")
	}
	if result.Count != 2 {
		t.Errorf("expected count=2, got %d", result.Count)
	}
	if result.Source != "test" {
		t.Errorf("expected source='test', got %q", result.Source)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}
}

func TestRenderJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	output.RenderJSON(&buf, []adapter.DataPoint{})

	var result output.JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if !result.OK {
		t.Error("expected ok=true for empty results")
	}
	if result.Count != 0 {
		t.Errorf("expected count=0, got %d", result.Count)
	}
}

func TestRenderJSONError(t *testing.T) {
	var buf bytes.Buffer
	output.RenderJSONError(&buf, "test", fmt.Errorf("something broke"))

	var result output.JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if result.OK {
		t.Error("expected ok=false")
	}
	if result.Error != "something broke" {
		t.Errorf("expected error message, got %q", result.Error)
	}
}

func TestRenderPlain(t *testing.T) {
	var buf bytes.Buffer
	points := samplePoints()

	output.RenderPlain(&buf, points)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	// Check tab separation.
	for i, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			t.Errorf("line %d: expected at least 3 tab-separated fields, got %d", i, len(parts))
		}
	}
}

func TestRenderPlainEmpty(t *testing.T) {
	var buf bytes.Buffer
	output.RenderPlain(&buf, []adapter.DataPoint{})

	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty points, got %q", buf.String())
	}
}

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	points := samplePoints()

	output.RenderTable(&buf, points, true) // no-color for testing

	out := buf.String()
	if !strings.Contains(out, "First Item") {
		t.Error("expected table to contain 'First Item'")
	}
	if !strings.Contains(out, "Second Item") {
		t.Error("expected table to contain 'Second Item'")
	}
	if !strings.Contains(out, "news/test") {
		t.Error("expected table to contain source info 'news/test'")
	}
}

func TestRenderTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	output.RenderTable(&buf, []adapter.DataPoint{}, true)

	if !strings.Contains(buf.String(), "No results") {
		t.Error("expected 'No results' message")
	}
}
