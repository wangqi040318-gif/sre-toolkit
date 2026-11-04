package slo

import "time"

// DemoBurns is a checkout service that is paging: the 1h window is at 20x,
// the 6h window is at 8x, and the 3d budget is already overspent.
func DemoBurns(service string) []Burn {
	if service == "" {
		service = "checkout"
	}
	specs := []struct {
		label string
		d     time.Duration
		ratio float64
	}{
		{"1h", time.Hour, 0.02},
		{"6h", 6 * time.Hour, 0.008},
		{"3d", 72 * time.Hour, 0.0015},
	}
	out := make([]Burn, len(specs))
	for i, sp := range specs {
		q, err := (Templates{}).Render(service, []Window{{Label: sp.label, Duration: sp.d}})
		query := ""
		if err == nil && len(q) == 1 {
			query = q[0]
		}
		out[i] = Burn{
			Window:     Window{Label: sp.label, Duration: sp.d},
			ErrorRatio: sp.ratio,
			Query:      query,
		}
	}
	return out
}
