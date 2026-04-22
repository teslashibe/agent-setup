package mcp

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

// ResponseShaper applies token-efficiency caps uniformly to every tool
// response, regardless of what the underlying tool returned.
//
//   - MaxItemsPerPage caps any list-shaped field at this many items and sets
//     "truncated": true on the parent object.
//   - MaxStringLen truncates any string longer than this many runes,
//     appending "…".
//   - MaxResponseBytes is the upper bound on the compact-JSON byte size of
//     the entire response. When exceeded, the shaper progressively halves
//     MaxStringLen and MaxItemsPerPage until the response fits, finally
//     replacing the response with a structured "_truncated" marker if it
//     still doesn't fit.
//
// Zero values disable individual caps.
type ResponseShaper struct {
	MaxItemsPerPage  int
	MaxStringLen     int
	MaxResponseBytes int
}

// Shape applies the configured caps to v and returns the shaped value.
// Mutation is in-place where possible; callers should treat v as consumed.
func (s ResponseShaper) Shape(v any) any {
	v = s.shape(v, s.MaxStringLen, s.MaxItemsPerPage)
	if s.MaxResponseBytes <= 0 {
		return v
	}
	str := s.MaxStringLen
	items := s.MaxItemsPerPage
	for i := 0; i < 4; i++ {
		buf, err := compactJSON(v)
		if err != nil || len(buf) <= s.MaxResponseBytes {
			return v
		}
		if str <= 100 && items <= 5 {
			break
		}
		if str > 100 {
			str /= 2
		}
		if items > 5 {
			items /= 2
		}
		v = s.shape(v, str, items)
	}
	buf, err := compactJSON(v)
	if err == nil && len(buf) <= s.MaxResponseBytes {
		return v
	}
	return map[string]any{
		"_truncated":   true,
		"_reason":      "response exceeded max bytes after iterative shaping",
		"_max_bytes":   s.MaxResponseBytes,
		"_actual_size": len(buf),
	}
}

func (s ResponseShaper) shape(v any, maxStr, maxItems int) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return truncateRunes(x, maxStr)
	case bool, float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		json.Number:
		return x
	case []any:
		if maxItems > 0 && len(x) > maxItems {
			x = x[:maxItems]
		}
		for i := range x {
			x[i] = s.shape(x[i], maxStr, maxItems)
		}
		return x
	case map[string]any:
		// Detect "page" shape (mcptool.Page wraps results as `items: [...]`)
		// BEFORE recursing into children, because recursion would already
		// truncate the items slice and we'd lose the signal to set
		// `truncated: true` on the parent.
		if items, ok := x["items"].([]any); ok && maxItems > 0 && len(items) > maxItems {
			x["truncated"] = true
		}
		for k, vv := range x {
			x[k] = s.shape(vv, maxStr, maxItems)
		}
		return x
	default:
		return s.shapeReflectFallback(v, maxStr, maxItems)
	}
}

// shapeReflectFallback handles concrete Go structs by round-tripping through
// JSON. This is correct for arbitrary Go values returned by tool handlers
// without forcing them to construct map[string]any.
//
// We strip any non-generic value before re-shaping so we never recurse back
// into shapeReflectFallback for a primitive whose type isn't in the type
// switch above (e.g. a custom string alias). After json.Unmarshal the
// resulting tree only contains map/slice/string/float64/bool/nil, all of
// which the shape() switch handles directly.
func (s ResponseShaper) shapeReflectFallback(v any, maxStr, maxItems int) any {
	buf, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var generic any
	if err := json.Unmarshal(buf, &generic); err != nil {
		return v
	}
	return s.shape(generic, maxStr, maxItems)
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func compactJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}
