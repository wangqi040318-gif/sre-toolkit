package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/triage"
)

type triageFlags struct {
	service  string
	window   time.Duration
	since    time.Duration
	until    string
	promURL  string
	token    string
	selector string
	requests string
	errors   string
	p95      string
	p99      string
	timeout  time.Duration
	demo     bool
}

func newTriage(opts *output.Options) *cobra.Command {
	flags := &triageFlags{}
	cmd := &cobra.Command{
		Use:   "triage [service]",
		Short: "On-call brief from Prometheus RED metrics",
		Long: `Build a short incident brief for one service from the Prometheus HTTP API.

The brief reports request rate, 5xx ratio, and latency p95/p99 over a window,
then a hypothesis and the next kubectl and PromQL checks. Logs are not queried.

Default PromQL assumes:
  - job label equals the service name
  - counter http_requests_total with label code=~"5.." for failures
  - histogram http_request_duration_seconds_bucket

Override the selector or any query. {service} and {selector} are substituted.
Set the URL with --prometheus-url or PROMETHEUS_URL. A bearer token may be
set with --bearer-token or PROMETHEUS_TOKEN and is not printed.

--window or --since is the lookback (default 1h). --until is an RFC3339 end
time (default now). --demo is an offline checkout spike and does not call Prometheus.

用 Prometheus 的 RED 指标生成值班排查简报。
--demo 是一次离线的错误率和延迟尖峰，不访问 Prometheus。`,
		Example: `  sre triage --demo
  sre triage --demo checkout --json
  sre triage checkout --prometheus-url http://prometheus:9090 --window 1h
  sre triage checkout --since 30m --selector 'service="{service}"'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTriage(cmd, opts, flags, args)
		},
	}
	cmd.Flags().StringVar(&flags.service, "service", "", "Service or Prometheus job name (alternative to the argument)")
	cmd.Flags().DurationVar(&flags.window, "window", 0, "Lookback window (default 1h)")
	cmd.Flags().DurationVar(&flags.since, "since", 0, "Lookback duration; use this or --window")
	cmd.Flags().StringVar(&flags.until, "until", "", "Window end as RFC3339 (default now)")
	cmd.Flags().StringVar(&flags.promURL, "prometheus-url", "", "Prometheus base URL (default: PROMETHEUS_URL)")
	cmd.Flags().StringVar(&flags.token, "bearer-token", "", "Prometheus bearer token (default: PROMETHEUS_TOKEN); not printed")
	cmd.Flags().StringVar(&flags.selector, "selector", "", "Label selector template inside the default queries (default job=\"{service}\")")
	cmd.Flags().StringVar(&flags.requests, "requests", "", "PromQL template for request rate")
	cmd.Flags().StringVar(&flags.errors, "errors", "", "PromQL template for the 5xx ratio")
	cmd.Flags().StringVar(&flags.p95, "p95", "", "PromQL template for latency p95")
	cmd.Flags().StringVar(&flags.p99, "p99", "", "PromQL template for latency p99")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 15*time.Second, "Prometheus HTTP timeout")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Use a synthetic error and latency spike and do not call Prometheus")
	return cmd
}

func (f triageFlags) validate(arg string) (string, time.Duration, time.Time, error) {
	service := f.service
	if arg != "" && service != "" && arg != service {
		return "", 0, time.Time{}, fmt.Errorf("argument %q and --service %q disagree", arg, service)
	}
	if service == "" {
		service = arg
	}
	if service == "" {
		if f.demo {
			service = "checkout"
		} else {
			return "", 0, time.Time{}, fmt.Errorf("service name is required (argument or --service), or pass --demo")
		}
	}
	if f.window > 0 && f.since > 0 {
		return "", 0, time.Time{}, fmt.Errorf("use either --window or --since")
	}
	lookback := f.window
	if f.since > 0 {
		lookback = f.since
	}
	if lookback == 0 {
		lookback = time.Hour
	}
	if lookback < time.Minute || lookback > 7*24*time.Hour {
		return "", 0, time.Time{}, fmt.Errorf("--window/--since must be between 1m and 168h")
	}
	end := time.Now().UTC().Truncate(time.Second)
	if strings.TrimSpace(f.until) != "" {
		parsed, err := time.Parse(time.RFC3339, f.until)
		if err != nil {
			return "", 0, time.Time{}, fmt.Errorf("--until must be RFC3339: %w", err)
		}
		end = parsed.UTC()
	}
	if f.timeout < time.Second || f.timeout > 2*time.Minute {
		return "", 0, time.Time{}, fmt.Errorf("--timeout must be between 1s and 2m")
	}
	return service, lookback, end, nil
}

func runTriage(cmd *cobra.Command, opts *output.Options, flags *triageFlags, args []string) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	arg := ""
	if len(args) == 1 {
		arg = args[0]
	}
	service, lookback, end, err := flags.validate(arg)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	start := end.Add(-lookback)
	q, err := (triage.Templates{
		Selector: flags.selector,
		Requests: flags.requests,
		Errors:   flags.errors,
		P95:      flags.p95,
		P99:      flags.p99,
	}).Render(service)
	if err != nil {
		return err
	}

	var bundle triage.Bundle
	mode := "prometheus"
	if flags.demo {
		bundle = triage.WithQueries(triage.DemoBundle(service, end), q)
		mode = "demo"
	} else {
		base := strings.TrimSpace(flags.promURL)
		if base == "" {
			base = strings.TrimSpace(os.Getenv("PROMETHEUS_URL"))
		}
		if base == "" {
			return fmt.Errorf("prometheus URL is required (--prometheus-url or PROMETHEUS_URL), or pass --demo")
		}
		token := flags.token
		if token == "" {
			token = os.Getenv("PROMETHEUS_TOKEN")
		}
		client := &triage.Client{
			BaseURL: base,
			Token:   token,
			HTTP:    &http.Client{Timeout: flags.timeout},
		}
		ctx, cancel := context.WithTimeout(context.Background(), flags.timeout)
		defer cancel()
		bundle, err = fetchBundle(ctx, client, service, start, end, q)
		if err != nil {
			return err
		}
	}
	rep := triage.Assemble(bundle, q)
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, triageDocument(rep, mode))
}

func fetchBundle(ctx context.Context, client *triage.Client, service string, start, end time.Time, q triage.Queries) (triage.Bundle, error) {
	step := triage.Step(end.Sub(start))
	type item struct {
		name  string
		query string
		dest  *triage.Signal
	}
	b := triage.Bundle{Service: service, Start: start, End: end}
	items := []item{
		{"requests", q.Requests, &b.Requests},
		{"errors", q.Errors, &b.Errors},
		{"p95", q.P95, &b.P95},
		{"p99", q.P99, &b.P99},
	}
	failures := 0
	var first error
	for _, it := range items {
		it.dest.Name = it.name
		it.dest.Query = it.query
		series, err := client.QueryRange(ctx, it.query, start, end, step)
		if err != nil {
			failures++
			if first == nil {
				first = err
			}
			it.dest.Err = err.Error()
			continue
		}
		chosen, ok := triage.Primary(series)
		if !ok {
			it.dest.Missing = true
			continue
		}
		it.dest.Series = chosen
	}
	if failures == len(items) && first != nil {
		return b, first
	}
	return b, nil
}
