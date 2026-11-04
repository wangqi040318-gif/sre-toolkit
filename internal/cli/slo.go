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
	"github.com/wangqi040318-gif/sre-toolkit/internal/slo"
	"github.com/wangqi040318-gif/sre-toolkit/internal/triage"
)

type sloFlags struct {
	service  string
	target   float64
	windows  string
	promURL  string
	token    string
	selector string
	errors   string
	timeout  time.Duration
	demo     bool
}

func newSLO(opts *output.Options) *cobra.Command {
	flags := &sloFlags{}
	cmd := &cobra.Command{
		Use:   "slo [service]",
		Short: "Availability error budget and multi-window burn rate",
		Long: `Compute an availability SLO error budget and burn rates from Prometheus.

The default target is 99.9%. Pass --slo 99.9 or --slo 0.999. --window is a
list of at least two Prometheus durations, default 1h,6h,3d (slash form
1h/6h/3d is also accepted). Burn rate is the window error ratio divided by
(1 - SLO). A fast burn at or above 14.4x pages. A 6h burn at or above 6x,
or a multi-day burn at or above 1x, freezes non-urgent releases.

Default PromQL matches sre triage: increase() of http_requests_total with
code=~"5.." over all requests, selector job="{service}". {window}, {service},
and {selector} are substituted. The URL is --prometheus-url or PROMETHEUS_URL.
A bearer token (--bearer-token or PROMETHEUS_TOKEN) is not printed.

--demo is an offline checkout budget that is already paging. It does not call Prometheus.

根据 Prometheus 计算可用性错误预算和多窗口燃烧率，并给出是否冻结发布。
--demo 是一次离线的预算烧穿，不访问 Prometheus。`,
		Example: `  sre slo --demo
  sre slo --demo --json
  sre slo checkout --prometheus-url http://prometheus:9090
  sre slo checkout --slo 99.95 --window 5m,1h,6h`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSLO(cmd, opts, flags, args)
		},
	}
	cmd.Flags().StringVar(&flags.service, "service", "", "Service or Prometheus job name (alternative to the argument)")
	cmd.Flags().Float64Var(&flags.target, "slo", 99.9, "Availability target as a percentage (99.9) or a ratio (0.999)")
	cmd.Flags().StringVar(&flags.windows, "window", "1h,6h,3d", "Comma or slash separated burn windows, at least two")
	cmd.Flags().StringVar(&flags.promURL, "prometheus-url", "", "Prometheus base URL (default: PROMETHEUS_URL)")
	cmd.Flags().StringVar(&flags.token, "bearer-token", "", "Prometheus bearer token (default: PROMETHEUS_TOKEN); not printed")
	cmd.Flags().StringVar(&flags.selector, "selector", "", "Label selector template (default job=\"{service}\")")
	cmd.Flags().StringVar(&flags.errors, "errors", "", "PromQL template for the error ratio; {window} is substituted")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 15*time.Second, "Prometheus HTTP timeout")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Use a synthetic burning budget and do not call Prometheus")
	return cmd
}

func (f sloFlags) validate(arg string) (string, float64, []slo.Window, error) {
	service := f.service
	if arg != "" && service != "" && arg != service {
		return "", 0, nil, fmt.Errorf("argument %q and --service %q disagree", arg, service)
	}
	if service == "" {
		service = arg
	}
	if service == "" {
		if f.demo {
			service = "checkout"
		} else {
			return "", 0, nil, fmt.Errorf("service name is required (argument or --service), or pass --demo")
		}
	}
	target, err := slo.ParseSLO(f.target)
	if err != nil {
		return "", 0, nil, err
	}
	windows, err := slo.ParseWindows(f.windows)
	if err != nil {
		return "", 0, nil, err
	}
	if f.timeout < time.Second || f.timeout > 2*time.Minute {
		return "", 0, nil, fmt.Errorf("--timeout must be between 1s and 2m")
	}
	return service, target, windows, nil
}

func runSLO(cmd *cobra.Command, opts *output.Options, flags *sloFlags, args []string) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	arg := ""
	if len(args) == 1 {
		arg = args[0]
	}
	service, target, windows, err := flags.validate(arg)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	tmpl := slo.Templates{Selector: flags.selector, Errors: flags.errors}
	var burns []slo.Burn
	mode := "prometheus"
	if flags.demo {
		burns = slo.DemoBurns(service)
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
		client := &triage.Client{BaseURL: base, Token: token, HTTP: &http.Client{Timeout: flags.timeout}}
		ctx, cancel := context.WithTimeout(context.Background(), flags.timeout)
		defer cancel()
		burns, err = slo.Fetch(ctx, client, service, windows, tmpl, time.Now().UTC())
		if err != nil {
			return err
		}
	}
	rep := slo.Assemble(service, target, burns, flags.demo)
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, sloDocument(rep, mode))
}
