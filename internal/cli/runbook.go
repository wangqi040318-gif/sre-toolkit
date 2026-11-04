package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/kube"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/runbook"
)

type runbookFlags struct {
	namespace  string
	name       string
	kind       string
	revision   int
	demo       bool
	kubeconfig string
	context    string
	timeout    time.Duration
}

func newRunbook(opts *output.Options) *cobra.Command {
	flags := &runbookFlags{}
	cmd := &cobra.Command{
		Use:   "runbook [name]",
		Short: "Rollback runbook for a Deployment or StatefulSet",
		Long: `Print an on-call rollback checklist for one Deployment or StatefulSet.

The checklist includes the current image, replicas, and revision, pre-checks,
kubectl rollout undo and set image commands, post-verify steps, and abort
criteria. Commands are filled with the namespace and are not executed.

Pass the workload name as an argument or with --name. --revision selects the
undo target; the default is the newest recorded revision below the current one.
--demo is a failed checkout release and does not contact a cluster.

Live mode reads the workload, its ReplicaSets or ControllerRevisions, and
matching pods through client-go.

生成 Deployment 或 StatefulSet 的回滚手册。
--demo 是一次失败的 checkout 发布，不访问集群。`,
		Example: `  sre runbook --demo
  sre runbook --demo --revision 11 --markdown
  sre runbook -n shop checkout
  sre runbook -n shop checkout --revision 11 --report rollback.md
  sre runbook -n data db --kind statefulset`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRunbook(cmd, opts, flags, args)
		},
	}
	cmd.Flags().StringVarP(&flags.namespace, "namespace", "n", "", "Namespace of the workload (default: kubeconfig namespace)")
	cmd.Flags().StringVar(&flags.name, "name", "", "Workload name (alternative to the positional argument)")
	cmd.Flags().StringVar(&flags.kind, "kind", "deployment", "Workload kind: deployment or statefulset")
	cmd.Flags().IntVar(&flags.revision, "revision", 0, "Revision to roll back to (default: the revision before current)")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Print the built-in failed checkout rollback and do not contact a cluster")
	cmd.Flags().StringVar(&flags.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file (default: KUBECONFIG or ~/.kube/config)")
	cmd.Flags().StringVar(&flags.context, "context", "", "Kubeconfig context (default: current context)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Second, "API request timeout")
	return cmd
}

func (f runbookFlags) validate(argName string) (string, string, error) {
	name := f.name
	if argName != "" && name != "" && argName != name {
		return "", "", fmt.Errorf("argument %q and --name %q disagree", argName, name)
	}
	if name == "" {
		name = argName
	}
	if !f.demo && name == "" {
		return "", "", fmt.Errorf("workload name is required (argument or --name), or pass --demo")
	}
	kind, err := normalizeKind(f.kind)
	if err != nil {
		return "", "", err
	}
	if f.demo && kind == runbook.KindStatefulSet && !kindWasDefault(f.kind) {
		return "", "", fmt.Errorf("the demo runbook is Deployment shop/checkout; omit --kind or set deployment")
	}
	if f.namespace != "" && !validNamespace(f.namespace) {
		return "", "", fmt.Errorf("invalid namespace %q", f.namespace)
	}
	if f.revision < 0 {
		return "", "", fmt.Errorf("--revision must be >= 0")
	}
	if f.timeout < time.Second || f.timeout > 10*time.Minute {
		return "", "", fmt.Errorf("--timeout must be between 1s and 10m")
	}
	return name, kind, nil
}

func kindWasDefault(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "deployment", "deploy":
		return true
	default:
		return false
	}
}

func normalizeKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "deployment", "deploy":
		return runbook.KindDeployment, nil
	case "statefulset", "sts":
		return runbook.KindStatefulSet, nil
	default:
		return "", fmt.Errorf("--kind must be deployment or statefulset")
	}
}

func runRunbook(cmd *cobra.Command, opts *output.Options, flags *runbookFlags, args []string) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	argName := ""
	if len(args) == 1 {
		argName = args[0]
	}
	name, kind, err := flags.validate(argName)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	var snap runbook.Snapshot
	mode := "demo"
	if flags.demo {
		snap = runbook.DemoSnapshot()
		if flags.namespace != "" {
			snap.Namespace = flags.namespace
		}
		if name != "" {
			snap.Name = name
			snap.Selector = "app=" + name
		}
		mode = "demo"
	} else {
		loaded, err := kube.Load(kube.Options{Kubeconfig: flags.kubeconfig, Context: flags.context})
		if err != nil {
			return err
		}
		ns := flags.namespace
		if ns == "" {
			ns = loaded.Namespace
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
		snap, err = runbook.Collect(ctx, client, kind, ns, name)
		if err != nil {
			return err
		}
		snap.Cluster = loaded.Context
		if snap.Cluster == "" {
			snap.Cluster = "unknown"
		}
		mode = loaded.Source
	}

	rep, err := runbook.Build(snap, runbook.Options{Revision: int64(flags.revision), Demo: flags.demo})
	if err != nil {
		return err
	}
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, runbookDocument(rep, mode))
}
