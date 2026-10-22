package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestsReadsDirectoriesDeterministically(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("kind: Service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.yml"), []byte("kind: Deployment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifests(context.Background(), []string{dir, filepath.Join(dir, "a.yml")})
	if err != nil {
		t.Fatal(err)
	}
	text := string(manifest)
	if strings.Index(text, "Deployment") > strings.Index(text, "Service") {
		t.Fatalf("manifest order is not deterministic: %s", text)
	}
	if strings.Contains(text, "ignored") {
		t.Fatal("non-YAML file was loaded")
	}
	if strings.Count(text, "Deployment") != 1 {
		t.Fatalf("overlapping paths loaded a manifest more than once: %s", text)
	}
}
