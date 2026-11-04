package diffreview

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func nested(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asMaps(v any) []map[string]any {
	switch s := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(s))
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return s
	default:
		return nil
	}
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func scalar(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return n
	case bool:
		return strconv.FormatBool(n)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		if n == math.Trunc(n) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case json.Number:
		return n.String()
	default:
		return compact(v)
	}
}

func scalarOr(v any, fallback string) string {
	s := scalar(v)
	if s == "" {
		return fallback
	}
	return s
}

func compact(v any) string {
	if v == nil {
		return ""
	}
	switch n := v.(type) {
	case string, bool, int, int64, float64, json.Number:
		return scalar(n)
	case []any:
		parts := make([]string, 0, len(n))
		for _, item := range n {
			parts = append(parts, scalar(item))
		}
		sort.Strings(parts)
		return strings.Join(parts, ",")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		s := string(b)
		if len(s) > 180 {
			return s[:177] + "..."
		}
		return s
	}
}
