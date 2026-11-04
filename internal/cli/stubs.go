package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
)

type plannedSpec struct {
	use   string
	short string
	long  string
	todo  string
}

func plannedSpecs() []plannedSpec {
	return []plannedSpec{
		{
			use:   "daily",
			short: "Daily operator digest (planned)",
			long: `One-page digest of overnight alerts, deploys, and pulse findings.

TODO: not implemented yet.

计划中：汇总隔夜告警、发布和 pulse 结果的日报。`,
			todo: "TODO: sre daily is not implemented yet. Planned: a one-page daily operator digest. 计划中：值班日报。",
		},
	}
}

func plannedCommand(opts *output.Options, spec plannedSpec) *cobra.Command {
	return &cobra.Command{
		Use:   spec.use,
		Short: spec.short,
		Long:  spec.long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := opts.Validate(); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			doc := output.Document{
				Title:       spec.use,
				Command:     spec.use,
				GeneratedAt: time.Now().UTC(),
				Notes:       []string{spec.todo},
			}
			return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, doc)
		},
	}
}
