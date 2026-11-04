package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExplicitContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(fixtureKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(Options{Kubeconfig: path})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Context != "payments" || loaded.Namespace != "payments" || loaded.Source != "kubeconfig" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.RestConfig == nil || loaded.RestConfig.Host != "https://kubernetes.example.invalid" {
		t.Fatalf("host = %+v", loaded.RestConfig)
	}
	if !strings.Contains(loaded.RestConfig.UserAgent, "sre-toolkit/") {
		t.Fatalf("user agent = %s", loaded.RestConfig.UserAgent)
	}

	shop, err := Load(Options{Kubeconfig: path, Context: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if shop.Context != "shop" || shop.Namespace != "shop" {
		t.Fatalf("shop = %+v", shop)
	}
}

func TestLoadMissingKubeconfig(t *testing.T) {
	_, err := Load(Options{Kubeconfig: filepath.Join(t.TempDir(), "missing")})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadNoDefaultAndNotInCluster(t *testing.T) {
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		t.Skip("in-cluster credentials are present")
	}
	t.Setenv("KUBECONFIG", "")
	t.Setenv("HOME", t.TempDir())
	_, err := Load(Options{})
	if err == nil {
		t.Fatal("expected error without kubeconfig")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no kubeconfig") || !strings.Contains(msg, "in-cluster") {
		t.Fatalf("err = %s", msg)
	}
}

const fixtureKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: demo
  cluster:
    server: https://kubernetes.example.invalid
    insecure-skip-tls-verify: true
contexts:
- name: payments
  context:
    cluster: demo
    user: demo
    namespace: payments
- name: shop
  context:
    cluster: demo
    user: demo
    namespace: shop
current-context: payments
users:
- name: demo
  user: {}
`
