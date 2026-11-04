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
			use:   "diff-review",
			short: "Compare manifests and summarize operational risk (planned)",
			long: `Compare two Kubernetes manifests, Helm renders, or live objects and
summarize what changed and why it matters operationally.

TODO: not implemented yet.

计划中：对比两份清单或线上对象，并说明变更的运维影响。`,
			todo: "TODO: sre diff-review is not implemented yet. Planned: compare two manifests or live objects and summarize operational risk. 计划中：对比清单或线上对象并说明运维影响。",
		},
		{
			use:   "triage",
			short: "Guided incident triage from a symptom (planned)",
			long: `Start from a pod, node, or error text and walk the next checks:
scope, recent changes, dependencies, and a suggested owner.

TODO: not implemented yet.

计划中：从 Pod、节点或报错出发，给出分层排查步骤。`,
			todo: "TODO: sre triage is not implemented yet. Planned: guided incident triage from a pod, node, or error text. 计划中：从现象出发的分层排查。",
		},
		{
			use:   "slo",
			short: "SLO burn and error-budget snapshot (planned)",
			long: `Show burn rate and remaining error budget for recorded SLIs.

TODO: not implemented yet.

计划中：展示错误预算消耗与燃烧率。`,
			todo: "TODO: sre slo is not implemented yet. Planned: error-budget and burn-rate snapshot. 计划中：错误预算与燃烧率快照。",
		},
		{
			use:   "runbook",
			short: "Fill a local runbook with cluster facts (planned)",
			long: `Render a local runbook template with cluster facts filled in.

TODO: not implemented yet.

计划中：用集群事实填充本地 runbook 模板。`,
			todo: "TODO: sre runbook is not implemented yet. Planned: render a local runbook with cluster facts filled in. 计划中：填充本地 runbook。",
		},
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
