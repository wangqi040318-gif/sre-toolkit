package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/kube"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/pulse"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type pulseFlags struct {
	namespace  string
	all        bool
	demo       bool
	kubeconfig string
	context    string
	threshold  int
	since      time.Duration
	eventLimit int
	timeout    time.Duration
}

func newPulse(opts *output.Options) *cobra.Command {
	flags := &pulseFlags{}
	cmd := &cobra.Command{
		Use:     "k8s-pulse",
		Aliases: []string{"pulse"},
		Short:   "Health pulse for pods, nodes, and warning events",
		Long: `Read nodes, pods, and warning events and report each signal as
phenomenon, likely cause, and next step.

Signals:
  - CrashLoopBackOff
  - OOMKilled (preferred over CrashLoop when the last exit was OOM)
  - Pods whose restart count is at or above --restart-threshold
  - Nodes whose Ready condition is not True
  - Warning events in the --since window

Live mode uses kubeconfig (KUBECONFIG or ~/.kube/config) or an in-cluster
service account. It lists the current context namespace unless -A is set.
--demo uses built-in data and does not contact a cluster. Demo mode scans
all namespaces unless --namespace is set.

Nodes are cluster-scoped and are included even with --namespace.

kubectl 风格的健康巡检：现象、可能原因、下一步。
--demo 使用内置数据，不访问集群。`,
		Example: `  sre k8s-pulse --demo
  sre k8s-pulse --demo --json
  sre k8s-pulse --demo --report pulse.md
  sre k8s-pulse -A
  sre k8s-pulse -n payments --since 30m`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPulse(cmd, opts, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.namespace, "namespace", "n", "", "Namespace to scan (default: kubeconfig namespace, or all in --demo)")
	cmd.Flags().BoolVarP(&flags.all, "all-namespaces", "A", false, "Scan pods and events in every namespace")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Use synthetic data and do not contact a cluster")
	cmd.Flags().StringVar(&flags.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file (default: KUBECONFIG or ~/.kube/config)")
	cmd.Flags().StringVar(&flags.context, "context", "", "Kubeconfig context (default: current context)")
	cmd.Flags().IntVar(&flags.threshold, "restart-threshold", 5, "Flag a pod at or above this restart count when it is not crash-looping or OOMKilled")
	cmd.Flags().DurationVar(&flags.since, "since", time.Hour, "Include warning events newer than this")
	cmd.Flags().IntVar(&flags.eventLimit, "event-limit", 20, "Maximum warning events to print")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Second, "API request timeout")
	return cmd
}

func (f pulseFlags) validate() error {
	if f.all && f.namespace != "" {
		return fmt.Errorf("use either --namespace or --all-namespaces, not both")
	}
	if f.namespace != "" && !validNamespace(f.namespace) {
		return fmt.Errorf("invalid namespace %q", f.namespace)
	}
	if f.threshold < 1 || f.threshold > 1_000_000 {
		return fmt.Errorf("--restart-threshold must be between 1 and 1000000")
	}
	if f.eventLimit < 1 || f.eventLimit > 1000 {
		return fmt.Errorf("--event-limit must be between 1 and 1000")
	}
	if f.since < time.Second || f.since > 7*24*time.Hour {
		return fmt.Errorf("--since must be between 1s and 168h")
	}
	if f.timeout < time.Second || f.timeout > 10*time.Minute {
		return fmt.Errorf("--timeout must be between 1s and 10m")
	}
	return nil
}

func validNamespace(ns string) bool {
	if len(ns) > 63 {
		return false
	}
	return namespacePattern.MatchString(ns)
}

func runPulse(cmd *cobra.Command, opts *output.Options, flags *pulseFlags) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if err := flags.validate(); err != nil {
		return err
	}
	cmd.SilenceUsage = true

	now := time.Now().UTC().Truncate(time.Second)
	all := flags.all
	namespace := flags.namespace
	var (
		snap pulse.Snapshot
		mode string
	)
	if flags.demo {
		snap = pulse.DemoSnapshot(now)
		if !all && namespace == "" {
			all = true
		}
		mode = "demo"
	} else {
		loaded, err := kube.Load(kube.Options{Kubeconfig: flags.kubeconfig, Context: flags.context})
		if err != nil {
			return err
		}
		if !all && namespace == "" {
			namespace = loaded.Namespace
		}
		if loaded.RestConfig != nil {
			loaded.RestConfig.Timeout = flags.timeout
		}
		client, err := kube.NewClient(loaded)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, flags.timeout)
		defer cancel()
		snap, err = pulse.Collect(ctx, client, namespace, all)
		if err != nil {
			return err
		}
		snap.Cluster = loaded.Context
		if snap.Cluster == "" {
			snap.Cluster = "unknown"
		}
		mode = loaded.Source
	}

	rep := pulse.Analyze(snap, pulse.Options{
		Namespace:        namespace,
		AllNamespaces:    all,
		RestartThreshold: int32(flags.threshold),
		Since:            flags.since,
		EventLimit:       flags.eventLimit,
		Now:              now,
		Demo:             flags.demo,
	})
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, pulseDocument(rep, mode))
}
