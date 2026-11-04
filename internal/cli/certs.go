package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wangqi040318-gif/sre-toolkit/internal/certs"
	"github.com/wangqi040318-gif/sre-toolkit/internal/kube"
	"github.com/wangqi040318-gif/sre-toolkit/internal/output"
)

type certFlags struct {
	namespace    string
	all          bool
	demo         bool
	kubeconfig   string
	context      string
	timeout      time.Duration
	warnDays     int
	criticalDays int
	files        []string
	dirs         []string
}

func newCerts(opts *output.Options) *cobra.Command {
	flags := &certFlags{}
	cmd := &cobra.Command{
		Use:   "certs",
		Short: "Certificate expiry for Ingress TLS, secrets, and local PEMs",
		Long: `Scan Kubernetes Ingress TLS (spec.tls secretName and hosts), Secrets of
type kubernetes.io/tls, and optional local PEM files.

Each certificate is reported as resource, notAfter, severity, and a next
step. Severity is expired, critical (default ≤7d), warn (default ≤30d), or
ok. Thresholds are --critical-days and --warn-days.

Live mode uses kubeconfig or an in-cluster service account. Without -n or
-A it uses the current context namespace. --file and --dir alone do not
contact a cluster. --demo uses built-in certificates and does not contact
a cluster. Demo mode scans all namespaces unless --namespace is set.

Private keys are not printed.

检查 Ingress TLS、kubernetes.io/tls Secret 和本地 PEM 的到期时间。
--demo 使用内置证书，不访问集群。`,
		Example: `  sre certs --demo
  sre certs --demo --json
  sre certs --demo -n payments --critical-days 7 --warn-days 30
  sre certs -A
  sre certs --file tls.crt --dir ./pems`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCerts(cmd, opts, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.namespace, "namespace", "n", "", "Namespace to scan (default: kubeconfig namespace, or all in --demo)")
	cmd.Flags().BoolVarP(&flags.all, "all-namespaces", "A", false, "Scan every namespace")
	cmd.Flags().BoolVar(&flags.demo, "demo", false, "Use synthetic certificates and do not contact a cluster")
	cmd.Flags().StringVar(&flags.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file (default: KUBECONFIG or ~/.kube/config)")
	cmd.Flags().StringVar(&flags.context, "context", "", "Kubeconfig context (default: current context)")
	cmd.Flags().DurationVar(&flags.timeout, "timeout", 30*time.Second, "API request timeout")
	cmd.Flags().IntVar(&flags.warnDays, "warn-days", 30, "Warn when days remaining are at or below this value")
	cmd.Flags().IntVar(&flags.criticalDays, "critical-days", 7, "Critical when days remaining are at or below this value")
	cmd.Flags().StringArrayVar(&flags.files, "file", nil, "PEM file to scan (repeatable). Does not require a cluster when used alone")
	cmd.Flags().StringArrayVar(&flags.dirs, "dir", nil, "Directory of .pem, .crt, .cer, and .cert files (repeatable)")
	return cmd
}

func (f certFlags) validate() error {
	if f.all && f.namespace != "" {
		return fmt.Errorf("use either --namespace or --all-namespaces, not both")
	}
	if f.namespace != "" && !validNamespace(f.namespace) {
		return fmt.Errorf("invalid namespace %q", f.namespace)
	}
	if f.criticalDays < 0 || f.criticalDays > 3650 {
		return fmt.Errorf("--critical-days must be between 0 and 3650")
	}
	if f.warnDays < 1 || f.warnDays > 3650 {
		return fmt.Errorf("--warn-days must be between 1 and 3650")
	}
	if f.warnDays <= f.criticalDays {
		return fmt.Errorf("--warn-days must be greater than --critical-days")
	}
	if f.timeout < time.Second || f.timeout > 10*time.Minute {
		return fmt.Errorf("--timeout must be between 1s and 10m")
	}
	return nil
}

func (f certFlags) wantCluster() bool {
	if f.demo {
		return false
	}
	if f.all || f.namespace != "" || f.kubeconfig != "" || f.context != "" {
		return true
	}
	return len(f.files) == 0 && len(f.dirs) == 0
}

func runCerts(cmd *cobra.Command, opts *output.Options, flags *certFlags) error {
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
	var materials []certs.Material
	var notes []string
	mode := "local"
	cluster := ""

	if flags.demo {
		demo, err := certs.DemoMaterials(now)
		if err != nil {
			return err
		}
		materials = append(materials, demo...)
		if !all && namespace == "" {
			all = true
		}
		mode = "demo"
		cluster = "demo-cluster"
	}

	if len(flags.files) > 0 || len(flags.dirs) > 0 {
		loaded, fileNotes, err := certs.LoadPaths(flags.files, flags.dirs)
		if err != nil {
			return err
		}
		materials = append(materials, loaded...)
		notes = append(notes, fileNotes...)
		if mode == "local" {
			mode = "local"
		} else if mode == "demo" {
			mode = "demo+local"
		}
	}

	if flags.wantCluster() {
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
		clusterMats, err := certs.Collect(ctx, client, namespace, all)
		if err != nil {
			return err
		}
		materials = append(materials, clusterMats...)
		cluster = loaded.Context
		if cluster == "" {
			cluster = "unknown"
		}
		mode = loaded.Source
		if len(flags.files) > 0 || len(flags.dirs) > 0 {
			mode = loaded.Source + "+local"
		}
	}

	rep := certs.Analyze(materials, certs.Options{
		Namespace:     namespace,
		AllNamespaces: all,
		WarnDays:      flags.warnDays,
		CriticalDays:  flags.criticalDays,
		Now:           now,
		Demo:          flags.demo,
		Cluster:       cluster,
		FilesScoped:   len(flags.files) > 0 || len(flags.dirs) > 0,
	})
	rep.Notes = append(rep.Notes, notes...)
	return output.Render(cmd.OutOrStdout(), cmd.ErrOrStderr(), *opts, certsDocument(rep, mode))
}
