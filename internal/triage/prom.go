package triage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client calls the Prometheus HTTP API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// Sample is one timestamped value.
type Sample struct {
	Time  time.Time
	Value float64
}

// Series is one Prometheus result series.
type Series struct {
	Labels map[string]string
	Points []Sample
}

// Query runs an instant query.
func (c *Client) Query(ctx context.Context, query string, at time.Time) ([]Series, error) {
	q := url.Values{}
	q.Set("query", query)
	if !at.IsZero() {
		q.Set("time", strconv.FormatInt(at.Unix(), 10))
	}
	return c.get(ctx, "/api/v1/query", q)
}

// QueryRange runs a range query.
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	if step <= 0 {
		return nil, fmt.Errorf("step must be positive")
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.FormatFloat(step.Seconds(), 'f', 0, 64))
	return c.get(ctx, "/api/v1/query_range", q)
}

func (c *Client) get(ctx context.Context, path string, query url.Values) ([]Series, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("prometheus URL is empty")
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("prometheus URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read prometheus response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus HTTP %d: %s", resp.StatusCode, trimBody(body))
	}
	return ParseAPI(body)
}

// ParseAPI decodes a Prometheus /api/v1/query or query_range body.
func ParseAPI(body []byte) ([]Series, error) {
	var raw apiResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if raw.Status != "success" {
		msg := raw.Error
		if msg == "" {
			msg = "status " + raw.Status
		}
		if raw.ErrorType != "" {
			msg = raw.ErrorType + ": " + msg
		}
		return nil, fmt.Errorf("prometheus: %s", msg)
	}
	out := make([]Series, 0, len(raw.Data.Result))
	for _, item := range raw.Data.Result {
		s := Series{Labels: item.Metric}
		if len(item.Value) > 0 {
			pt, err := parsePoint(item.Value)
			if err != nil {
				return nil, err
			}
			if !math.IsNaN(pt.Value) {
				s.Points = append(s.Points, pt)
			}
		}
		for _, pair := range item.Values {
			pt, err := parsePoint(pair)
			if err != nil {
				return nil, err
			}
			if math.IsNaN(pt.Value) {
				continue
			}
			s.Points = append(s.Points, pt)
		}
		out = append(out, s)
	}
	return out, nil
}

// Primary returns the series whose last point is the largest.
// Multiple series usually mean the query was not fully aggregated; the worst
// series is the one an on-call brief should lead with.
func Primary(series []Series) (Series, bool) {
	var best Series
	ok := false
	var bestLast float64
	for _, s := range series {
		if len(s.Points) == 0 {
			continue
		}
		last := s.Points[len(s.Points)-1].Value
		if !ok || last > bestLast {
			best = s
			bestLast = last
			ok = true
		}
	}
	return best, ok
}

type apiResponse struct {
	Status    string `json:"status"`
	Error     string `json:"error"`
	ErrorType string `json:"errorType"`
	Data      struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
			Values [][]any           `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func parsePoint(raw []any) (Sample, error) {
	if len(raw) < 2 {
		return Sample{}, fmt.Errorf("prometheus sample has %d fields", len(raw))
	}
	ts, ok := asFloat(raw[0])
	if !ok {
		return Sample{}, fmt.Errorf("prometheus timestamp %v", raw[0])
	}
	val, ok := asFloat(raw[1])
	if !ok {
		text := fmt.Sprint(raw[1])
		if strings.EqualFold(text, "NaN") {
			return Sample{Time: time.Unix(int64(ts), 0).UTC(), Value: math.NaN()}, nil
		}
		return Sample{}, fmt.Errorf("prometheus value %v", raw[1])
	}
	return Sample{Time: time.Unix(int64(ts), 0).UTC(), Value: val}, nil
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func trimBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
