package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/diffreview"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
)

type diffFlags struct {
	before string
	after  string
	demo   bool
}

func newDiffReview(opts *output.Options) *cobra.Command {
	flags := &diffFlags{}
	cmd := &cobra.Command{
		Use:   "diff-review",
		Short: "Review risky field changes between two manifest sets",
		Long: `Compare two Kubernetes manifest sets and highlight release risks.

Compared fields:
  - container image
  - resource requests and limits
  - liveness, readiness, and startup probes
  - replica counts
  - HPA min, max, and metric target
  - volumes, volume mounts, and PVC claim names
  - Service type and ports
  - NetworkPolicy spec
  - Role, ClusterRole, and bindings
  - securityContext (privileged, runAsUser, capabilities, host namespaces)
  - env values and secretKeyRef / configMapKeyRef

--before and --after are files or directories of YAML or JSON. Render Helm
first (helm template) and pass the two output directories; this command
does not run Helm. --demo uses a built-in before/after pair and does not
read your files.

对比两份 Kubernetes 清单里的高风险字段，供发布评审。
--demo 使用内置的 before/after，不读取本地文件。`,
		Example: `  sre diff-review --demo
  sre diff-review --demo --json
  sre diff-review --before ./before --after ./after
  helm template rel chart -f old.yaml > /tmp/before.yaml
  helm template rel chart -f new.yaml > /tmp/after.yaml
  sre diff-review --before /tmp/before.yaml --after /tmp/after.yaml --report review.md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiffReview(cmd, opts, flags)
		},
	}
	cmd.Flags().StringVar(&flags.before, "before", "", "File or directory of manifests before the change")
	cmd.Flags().StringVar(&flags.after, "after", "", "File or directory of manifests after the change")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Review the built-in before/after pair and do not read local files")
	return cmd
}

func (f diffFlags) validate() error {
	if f.demo {
		return nil
	}
	if f.before == "" || f.after == "" {
		return fmt.Errorf("both --before and --after are required (or pass --demo)")
	}
	return nil
}

func runDiffReview(cmd *cobra.Command, opts *output.Options, flags *diffFlags) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if err := flags.validate(); err != nil {
		return err
	}
	cmd.SilenceUsage = true

	var (
		before, after []diffreview.Object
		err           error
		beforePath    = flags.before
		afterPath     = flags.after
	)
	if flags.demo {
		before, after, err = diffreview.DemoPair()
		beforePath, afterPath = "demo-before", "demo-after"
	} else {
		before, err = diffreview.LoadPath(flags.before)
		if err != nil {
			return err
		}
		if err == nil {
			after, err = diffreview.LoadPath(flags.after)
		}
	}
	if err != nil {
		return err
	}
	rep := diffreview.Compare(before, after, flags.demo, beforePath, afterPath)
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, diffDocument(rep))
}
