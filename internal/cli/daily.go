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

	"github.com/wangqi040318-gif/sre-toolkit/internal/daily"
	"github.com/wangqi040318-gif/sre-toolkit/internal/kube"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
)

type dailyFlags struct {
	contexts     []string
	contextsFile string
	kubeconfig   string
	timeout      time.Duration
	demo         bool
}

func newDaily(opts *output.Options) *cobra.Command {
	flags := &dailyFlags{}
	cmd := &cobra.Command{
		Use:   "daily",
		Short: "Morning health digest across kubeconfig contexts",
		Long: `Print a one-page digest for one or more clusters.

Each context reports NotReady nodes, CrashLoopBackOff and OOMKilled pods,
failed Jobs, recent Warning events, and certificates inside the sre certs
7-day and 30-day windows. Repeat --context or pass --contexts-file (one
name per line, # comments allowed). With neither, the current context is used.

--markdown is the form to paste into Slack or Feishu. --demo prints two
synthetic contexts, prod-eu and prod-us, and does not contact a cluster.

汇总多个集群的早报：节点、CrashLoop/OOM、失败 Job、Warning 事件和即将过期的证书。
--demo 是两个合成集群，不访问集群。`,
		Example: `  sre daily --demo
  sre daily --demo --markdown
  sre daily --context prod-eu --context prod-us --markdown
  sre daily --contexts-file contexts.txt --report digest.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDaily(cmd, opts, flags)
		},
	}
	cmd.Flags().StringArrayVar(&flags.contexts, "context", nil, "Kubeconfig context to include (repeatable)")
	cmd.Flags().StringVar(&flags.contextsFile, "contexts-file", "", "File of context names, one per line")
	cmd.Flags().StringVar(&flags.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file (default: KUBECONFIG or ~/.kube/config)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Second, "Per-context API timeout")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Print two synthetic clusters and do not contact a cluster")
	return cmd
}

func (f dailyFlags) validate() error {
	if f.timeout < time.Second || f.timeout > 10*time.Minute {
		return fmt.Errorf("--timeout must be between 1s and 10m")
	}
	return nil
}

func runDaily(cmd *cobra.Command, opts *output.Options, flags *dailyFlags) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if err := flags.validate(); err != nil {
		return err
	}
	cmd.SilenceUsage = true
	now := time.Now().UTC().Truncate(time.Second)
	var clusters []daily.Cluster
	mode := "demo"
	if flags.demo {
		clusters = daily.Demo(now)
	} else {
		names, err := daily.MergeContexts(flags.contexts, flags.contextsFile)
		if err != nil {
			return err
		}
		if len(names) == 0 {
			names = []string{""}
		}
		mode = "kubeconfig"
		var failed int
		for _, name := range names {
			loaded, err := kube.Load(kube.Options{Kubeconfig: flags.kubeconfig, Context: name})
			label := name
			if label == "" {
				label = "current"
			}
			if err != nil {
				failed++
				clusters = append(clusters, daily.Cluster{Context: label, Err: err.Error()})
				continue
			}
			if loaded.RestConfig != nil {
				loaded.RestConfig.Timeout = flags.timeout
			}
			client, err := kube.NewClient(loaded)
			if err != nil {
				failed++
				clusters = append(clusters, daily.Cluster{Context: loaded.Context, Err: err.Error()})
				continue
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			ctx, cancel := context.WithTimeout(ctx, flags.timeout)
			got, err := daily.CollectCluster(ctx, client, loaded.Context, now)
			cancel()
			stop()
			if err != nil {
				failed++
				ctxName := loaded.Context
				if ctxName == "" {
					ctxName = label
				}
				clusters = append(clusters, daily.Cluster{Context: ctxName, Err: err.Error()})
				continue
			}
			if got.Context == "" {
				got.Context = label
			}
			clusters = append(clusters, got)
		}
		if failed == len(clusters) {
			return fmt.Errorf("every context failed: %s", summarizeFailures(clusters))
		}
	}
	rep := daily.Digest(clusters, flags.demo)
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, dailyDocument(rep, mode))
}

func summarizeFailures(clusters []daily.Cluster) string {
	parts := make([]string, 0, len(clusters))
	for _, c := range clusters {
		parts = append(parts, c.Context+": "+c.Err)
	}
	return strings.Join(parts, "; ")
}
