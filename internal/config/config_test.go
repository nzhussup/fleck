package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesRelativePathsAndValidates(t *testing.T) {
	d := t.TempDir()
	for _, p := range []string{"cmd/api", "chart"} {
		if err := os.MkdirAll(filepath.Join(d, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(d, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "values.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := `version: 1
application: {language: go, path: ./cmd/api}
deployment:
  type: helm
  chart: ./chart
  release: app
  values: [./values.yaml]
  workload: {kind: Deployment, name: app}
  container: app
dockerfile: {path: ./Dockerfile}
analysis: {rules: [CFG001]}
`
	path := filepath.Join(d, "fleck.yaml")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Resolve(c.Application.Path); got != filepath.Join(d, "cmd/api") {
		t.Fatalf("resolved path=%s", got)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fleck.yaml")
	_ = os.WriteFile(p, []byte("version: 1\nunknown: true\n"), 0o644)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}
