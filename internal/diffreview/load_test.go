package diffreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(deployImage("ghcr.io/example/checkout:1.4.2", 2)), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "net")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "policy.yml"), []byte(netpol(false)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	objs, err := LoadPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Fatalf("objects = %d", len(objs))
	}
	if _, err := LoadPath(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("expected missing path error")
	}
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("kind: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPath(bad); err == nil {
		t.Fatal("expected yaml error")
	}
}

func TestDuplicateObject(t *testing.T) {
	dir := t.TempDir()
	body := deployImage("ghcr.io/example/checkout:1.4.2", 1)
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPath(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v", err)
	}
}
