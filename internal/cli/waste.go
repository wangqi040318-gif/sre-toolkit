package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/kube"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/waste"
)

type wasteFlags struct {
	namespace     string
	all           bool
	demo          bool
	kubeconfig    string
	context       string
	timeout       time.Duration
	idleFor       time.Duration
	usageRatio    float64
	includeConfig bool
}

func newWaste(opts *output.Options) *cobra.Command {
	flags := &wasteFlags{}
	cmd := &cobra.Command{
		Use:   "waste",
		Short: "Idle workloads, oversized requests, empty load balancers, and orphaned PVCs",
		Long: `Scan for Kubernetes spend that is not earning its keep.

Signals:
  - Deployments, StatefulSets, and bare ReplicaSets at 0 replicas, or with
    no Ready pods for longer than --idle-for
  - Requests far above metrics.k8s.io usage (under --usage-ratio, default 20%,
    and only when the request is at least 500m CPU or 512Mi memory)
  - Without metrics, requests of at least 2 CPU or 4Gi memory
  - LoadBalancer Services with no ready endpoints
  - Bound PVCs no running Pod mounts, and unbound PVCs
  - ConfigMaps and Secrets no Pod references (info only; service-account
    tokens, Helm releases, and kube-root-ca.crt are skipped)

ReplicaSets owned by a Deployment are not reported separately.
--demo uses built-in data and does not contact a cluster. Demo mode scans
all namespaces unless --namespace is set.

成本卫生巡检：闲置工作负载、过大的 requests、空载 LoadBalancer、无人挂载的 PVC。
--demo 使用内置数据，不访问集群。`,
		Example: `  sre waste --demo
  sre waste --demo --json
  sre waste --demo -n shop
  sre waste -A
  sre waste -n payments --idle-for 48h --usage-ratio 0.3`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWaste(cmd, opts, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.namespace, "namespace", "n", "", "Namespace to scan (default: kubeconfig namespace, or all in --demo)")
	cmd.Flags().BoolVarP(&flags.all, "all-namespaces", "A", false, "Scan every namespace")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Use synthetic findings and do not contact a cluster")
	cmd.Flags().StringVar(&flags.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file (default: KUBECONFIG or ~/.kube/config)")
	cmd.Flags().StringVar(&flags.context, "context", "", "Kubeconfig context (default: current context)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Second, "API request timeout")
	cmd.Flags().DurationVar(&flags.idleFor, "idle-for", 24*time.Hour, "Flag a workload with zero Ready pods only after this age")
	cmd.Flags().Float64Var(&flags.usageRatio, "usage-ratio", 0.20, "Flag requests whose usage is under this fraction of the request")
	cmd.Flags().BoolVar(&flags.includeConfig, "include-config", true, "Include unused ConfigMaps and Secrets (info severity)")
	return cmd
}

func (f wasteFlags) validate() error {
	if f.all && f.namespace != "" {
		return fmt.Errorf("use either --namespace or --all-namespaces, not both")
	}
	if f.namespace != "" && !validNamespace(f.namespace) {
		return fmt.Errorf("invalid namespace %q", f.namespace)
	}
	if f.idleFor < time.Minute || f.idleFor > 90*24*time.Hour {
		return fmt.Errorf("--idle-for must be between 1m and 2160h")
	}
	if f.usageRatio < 0.01 || f.usageRatio > 0.9 {
		return fmt.Errorf("--usage-ratio must be between 0.01 and 0.9")
	}
	if f.timeout < time.Second || f.timeout > 10*time.Minute {
		return fmt.Errorf("--timeout must be between 1s and 10m")
	}
	return nil
}

func runWaste(cmd *cobra.Command, opts *output.Options, flags *wasteFlags) error {
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
		snap waste.Snapshot
		mode string
	)
	if flags.demo {
		snap = waste.DemoSnapshot(now)
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
		var metrics waste.UsageSource
		var metricsBuildErr string
		if src, err := waste.NewUsageSource(loaded.RestConfig); err != nil {
			metricsBuildErr = err.Error()
		} else {
			metrics = src
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, flags.timeout)
		defer cancel()
		collected, err := waste.Collect(ctx, client, metrics, namespace, all)
		if err != nil {
			return err
		}
		if metricsBuildErr != "" {
			collected.MetricsAvailable = false
			collected.MetricsError = metricsBuildErr
		}
		snap = collected
		snap.Cluster = loaded.Context
		if snap.Cluster == "" {
			snap.Cluster = "unknown"
		}
		mode = loaded.Source
	}

	rep := waste.Analyze(snap, waste.Options{
		Namespace:     namespace,
		AllNamespaces: all,
		Now:           now,
		Demo:          flags.demo,
		Cluster:       snap.Cluster,
		IdleFor:       flags.idleFor,
		UsageRatio:    flags.usageRatio,
		IncludeConfig: flags.includeConfig,
	})
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, wasteDocument(rep, mode))
}
