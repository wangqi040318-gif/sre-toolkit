package slo

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/wangqi040318-gif/sre-toolkit/internal/triage"
)

// Fetch queries the error ratio for each window. A single empty window is
// recorded as missing. The call fails only when every window fails.
func Fetch(ctx context.Context, client *triage.Client, service string, windows []Window, tmpl Templates, at time.Time) ([]Burn, error) {
	queries, err := tmpl.Render(service, windows)
	if err != nil {
		return nil, err
	}
	out := make([]Burn, len(windows))
	failures := 0
	var first error
	for i, w := range windows {
		out[i] = Burn{Window: w, Query: queries[i]}
		series, err := client.Query(ctx, queries[i], at)
		if err != nil {
			failures++
			if first == nil {
				first = err
			}
			out[i].Err = err.Error()
			continue
		}
		chosen, ok := triage.Primary(series)
		if !ok || len(chosen.Points) == 0 || math.IsNaN(chosen.Points[len(chosen.Points)-1].Value) {
			out[i].Missing = true
			continue
		}
		out[i].ErrorRatio = chosen.Points[len(chosen.Points)-1].Value
	}
	if failures == len(windows) && first != nil {
		return out, fmt.Errorf("prometheus: %w", first)
	}
	return out, nil
}
