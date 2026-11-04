// Package kube loads a client-go config from kubeconfig or an in-cluster token.
package kube

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/wangqi040318-gif/sre-toolkit/internal/version"
)

// ErrNoKubeconfig is returned when no kubeconfig file is present and
// in-cluster configuration is also unavailable. Callers wrapping Load
// should still try in-cluster before surfacing this error; Load does that.
var ErrNoKubeconfig = errors.New("no kubeconfig file found")

// Options selects which kubeconfig and context to read.
type Options struct {
	Kubeconfig string
	Context    string
}

// Loaded is a ready-to-use API config plus the namespace from the context.
type Loaded struct {
	RestConfig *rest.Config
	Context    string
	Namespace  string
	Source     string
}

// Load reads an explicit kubeconfig, the default kubeconfig, or the
// in-cluster service account. An explicit --kubeconfig path never falls
// through to in-cluster config.
func Load(opts Options) (Loaded, error) {
	if strings.TrimSpace(opts.Kubeconfig) != "" {
		return loadExplicit(opts)
	}
	loaded, err := loadDefault(opts)
	if err == nil {
		return loaded, nil
	}
	if !errors.Is(err, ErrNoKubeconfig) {
		return Loaded{}, err
	}
	cfg, icErr := rest.InClusterConfig()
	if icErr != nil {
		return Loaded{}, fmt.Errorf("kubeconfig: %w; in-cluster config: %v", err, icErr)
	}
	prepare(cfg)
	return Loaded{
		RestConfig: cfg,
		Context:    "in-cluster",
		Namespace:  inClusterNamespace(),
		Source:     "in-cluster",
	}, nil
}

// NewClient builds a typed Kubernetes client.
func NewClient(loaded Loaded) (kubernetes.Interface, error) {
	if loaded.RestConfig == nil {
		return nil, errors.New("kubernetes rest config is nil")
	}
	client, err := kubernetes.NewForConfig(loaded.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return client, nil
}

func loadExplicit(opts Options) (Loaded, error) {
	if _, err := os.Stat(opts.Kubeconfig); err != nil {
		return Loaded{}, fmt.Errorf("kubeconfig %s: %w", opts.Kubeconfig, err)
	}
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: opts.Kubeconfig}
	return fromRules(rules, opts, "kubeconfig")
}

func loadDefault(opts Options) (Loaded, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if !defaultConfigExists(rules) {
		return Loaded{}, ErrNoKubeconfig
	}
	return fromRules(rules, opts, "kubeconfig")
}

func defaultConfigExists(rules *clientcmd.ClientConfigLoadingRules) bool {
	if env := strings.TrimSpace(os.Getenv("KUBECONFIG")); env != "" {
		return true
	}
	for _, path := range rules.GetLoadingPrecedence() {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func fromRules(rules clientcmd.ClientConfigLoader, opts Options, source string) (Loaded, error) {
	overrides := &clientcmd.ConfigOverrides{}
	if opts.Context != "" {
		overrides.CurrentContext = opts.Context
	}
	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	cfg, err := cc.ClientConfig()
	if err != nil {
		return Loaded{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	ns, _, err := cc.Namespace()
	if err != nil {
		return Loaded{}, fmt.Errorf("read namespace: %w", err)
	}
	if ns == "" {
		ns = "default"
	}
	raw, err := cc.RawConfig()
	if err != nil {
		return Loaded{}, fmt.Errorf("read kubeconfig: %w", err)
	}
	ctxName := opts.Context
	if ctxName == "" {
		ctxName = raw.CurrentContext
	}
	prepare(cfg)
	return Loaded{
		RestConfig: cfg,
		Context:    ctxName,
		Namespace:  ns,
		Source:     source,
	}, nil
}

func prepare(cfg *rest.Config) {
	if cfg == nil {
		return
	}
	cfg.UserAgent = version.UserAgent()
	cfg.QPS = 20
	cfg.Burst = 40
}

func inClusterNamespace() string {
	b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "default"
	}
	ns := strings.TrimSpace(string(b))
	if ns == "" {
		return "default"
	}
	return ns
}
