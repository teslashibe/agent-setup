package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestShape_StringTruncation guards the spec requirement that long strings
// are truncated to MaxStringLen runes with a trailing ellipsis.
func TestShape_StringTruncation(t *testing.T) {
	s := ResponseShaper{MaxStringLen: 10}
	long := strings.Repeat("a", 50)
	got := s.Shape(long)
	str, ok := got.(string)
	if !ok {
		t.Fatalf("want string, got %T", got)
	}
	if len([]rune(str)) != 10 {
		t.Fatalf("want 10 runes, got %d (%q)", len([]rune(str)), str)
	}
	if !strings.HasSuffix(str, "…") {
		t.Fatalf("want ellipsis suffix, got %q", str)
	}
}

// TestShape_ArrayCap caps top-level slices at MaxItemsPerPage.
func TestShape_ArrayCap(t *testing.T) {
	s := ResponseShaper{MaxItemsPerPage: 3}
	in := []any{1, 2, 3, 4, 5, 6, 7}
	got := s.Shape(in)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("want []any, got %T", got)
	}
	if len(arr) != 3 {
		t.Fatalf("want 3 items, got %d", len(arr))
	}
}

// TestShape_PageItemsTruncatedFlag sets `truncated:true` when an `items`
// field on a map is shortened. This is the contract mcptool.PageOf returns
// rely on for the agent to know it should re-call with a cursor.
func TestShape_PageItemsTruncatedFlag(t *testing.T) {
	s := ResponseShaper{MaxItemsPerPage: 2}
	in := map[string]any{
		"items":  []any{"a", "b", "c", "d"},
		"cursor": "next",
	}
	got := s.Shape(in).(map[string]any)
	items := got["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if got["truncated"] != true {
		t.Fatalf("want truncated=true, got %v", got["truncated"])
	}
}

// TestShape_StructFallback exercises the reflective JSON-roundtrip path so
// arbitrary Go values (the common case from scraper handlers) get capped.
func TestShape_StructFallback(t *testing.T) {
	type item struct {
		Title string `json:"title"`
	}
	type page struct {
		Items []item `json:"items"`
	}
	s := ResponseShaper{MaxItemsPerPage: 1, MaxStringLen: 5}
	in := page{Items: []item{
		{Title: "this is too long"},
		{Title: "second"},
		{Title: "third"},
	}}
	got := s.Shape(in)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any, got %T", got)
	}
	items := m["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if !strings.HasSuffix(first["title"].(string), "…") {
		t.Fatalf("want truncated string, got %q", first["title"])
	}
}

// TestShape_ByteCapHardCeiling ensures we never return more than
// MaxResponseBytes worth of compact-JSON, even if individual items are
// already at the per-string cap. The shaper iteratively halves caps and
// finally falls back to a structured marker.
func TestShape_ByteCapHardCeiling(t *testing.T) {
	s := ResponseShaper{MaxItemsPerPage: 50, MaxStringLen: 800, MaxResponseBytes: 256}
	huge := make([]any, 100)
	for i := range huge {
		huge[i] = strings.Repeat("x", 100)
	}
	got := s.Shape(huge)
	buf, err := compactJSON(got)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(buf) > s.MaxResponseBytes {
		t.Fatalf("output exceeded byte cap: %d > %d", len(buf), s.MaxResponseBytes)
	}
}

// TestCompactJSON_NoIndentation enforces the spec rule that responses are
// compact JSON (no newlines, no indentation). Critical for token-efficiency.
func TestCompactJSON_NoIndentation(t *testing.T) {
	in := map[string]any{"a": 1, "b": []int{2, 3}}
	buf, err := compactJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(buf), "\n  ") {
		t.Fatalf("compactJSON included whitespace: %q", buf)
	}
	var v map[string]any
	if err := json.Unmarshal(buf, &v); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}
