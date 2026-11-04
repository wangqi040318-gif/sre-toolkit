package triage

import "time"

// DemoBundle is a one-hour checkout incident: quiet for most of the hour,
// then 5xx and p95 spike in the last 15 minutes while request rate eases.
func DemoBundle(service string, end time.Time) Bundle {
	if service == "" {
		service = "checkout"
	}
	if end.IsZero() {
		end = time.Now().UTC().Truncate(time.Minute)
	}
	end = end.UTC()
	start := end.Add(-time.Hour)
	step := 5 * time.Minute
	return Bundle{
		Service:  service,
		Start:    start,
		End:      end,
		Demo:     true,
		Requests: Signal{Name: "requests", Series: seriesFrom(start, step, 120, 120, 118, 121, 119, 122, 120, 117, 116, 95, 80, 85, 90)},
		Errors:   Signal{Name: "errors", Series: seriesFrom(start, step, 0.002, 0.002, 0.003, 0.002, 0.002, 0.004, 0.003, 0.002, 0.003, 0.04, 0.09, 0.14, 0.12)},
		P95:      Signal{Name: "p95", Series: seriesFrom(start, step, 0.08, 0.07, 0.08, 0.09, 0.08, 0.08, 0.07, 0.08, 0.09, 0.40, 0.90, 1.40, 1.20)},
		P99:      Signal{Name: "p99", Series: seriesFrom(start, step, 0.15, 0.14, 0.16, 0.15, 0.15, 0.17, 0.16, 0.15, 0.18, 0.80, 1.80, 3.20, 2.80)},
	}
}

// WithQueries attaches the rendered PromQL so the brief can print the checks.
func WithQueries(b Bundle, q Queries) Bundle {
	b.Requests.Query = q.Requests
	b.Errors.Query = q.Errors
	b.P95.Query = q.P95
	b.P99.Query = q.P99
	return b
}

func seriesFrom(start time.Time, step time.Duration, values ...float64) Series {
	pts := make([]Sample, len(values))
	for i, v := range values {
		pts[i] = Sample{Time: start.Add(time.Duration(i) * step), Value: v}
	}
	return Series{Points: pts}
}
