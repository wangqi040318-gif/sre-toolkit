package certs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPaths(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	pemBytes, _, err := mintCert(now, "file.example", []string{"file.example"}, now.Add(40*24*time.Hour), 9)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "leaf.crt"), pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}
	hidden := filepath.Join(dir, ".git")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "skip.pem"), pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "extra.pem")
	if err := os.WriteFile(file, pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, notes, err := LoadPaths([]string{file}, []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Fatalf("notes = %v", notes)
	}
	// extra.pem once via --file and once via the directory walk.
	if len(loaded) != 3 {
		t.Fatalf("loaded = %d", len(loaded))
	}
	rep := Analyze(loaded, Options{AllNamespaces: true, Now: now, CriticalDays: 7, WarnDays: 30, FilesScoped: true})
	if rep.Summary.Certificates != 3 || rep.Summary.OK != 3 || rep.Summary.Files != 3 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if !strings.Contains(rep.Scope, "local files") {
		t.Fatalf("scope = %s", rep.Scope)
	}

	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	_, notes, err = LoadPaths(nil, []string{empty})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "No .pem") {
		t.Fatalf("notes = %v", notes)
	}
	if _, _, err := LoadPaths([]string{filepath.Join(dir, "missing.pem")}, nil); err == nil {
		t.Fatal("expected missing file error")
	}
}
