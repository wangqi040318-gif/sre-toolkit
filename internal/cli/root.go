// Package cli wires the sre command tree.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
	"github.com/wangqi040318-gif/sre-toolkit/internal/version"
)

// Execute runs the sre CLI.
func Execute() error {
	return NewRoot().Execute()
}

// NewRoot builds the command tree. Each call has its own flag state.
func NewRoot() *cobra.Command {
	opts := &output.Options{}
	root := &cobra.Command{
		Use:   "sre",
		Short: "SRE and cloud-native operations toolkit",
		Long: `sre is a kubectl-friendly toolkit for SRE and cloud-native operations.

Shipped commands are k8s-pulse, certs, and waste. Other commands are reserved
and print a TODO until later releases.

sre 是面向 SRE 与云原生运维的命令行工具。
当前已实现 k8s-pulse、certs 和 waste；其余子命令为占位，后续版本实现。`,
		SilenceErrors:     true,
		Version:           version.String(),
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
	}
	// Keep help in the order commands are registered: the shipped command,
	// the reserved roadmap, then version.
	cobra.EnableCommandSorting = false
	root.SetVersionTemplate("sre {{.Version}}\n")
	root.PersistentFlags().BoolVar(&opts.JSON, "json", false, "Print machine-readable JSON")
	root.PersistentFlags().BoolVar(&opts.Markdown, "markdown", false, "Print a Markdown report")
	root.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "Disable colored output")
	root.PersistentFlags().StringVar(&opts.Report, "report", "", "Also write a Markdown report to this file")

	root.AddCommand(newPulse(opts))
	root.AddCommand(newCerts(opts))
	root.AddCommand(newWaste(opts))
	for _, spec := range plannedSpecs() {
		root.AddCommand(plannedCommand(opts, spec))
	}
	root.AddCommand(newVersion())
	return root
}

func newVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sre version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "sre "+version.String())
		},
	}
}
