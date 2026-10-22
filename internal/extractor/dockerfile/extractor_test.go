package dockerfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFinalStageWithInheritance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	contents := "FROM alpine AS runtime\nARG DEFAULT_PORT=8080\nENV BASE_PORT=$DEFAULT_PORT APP_PORT=$BASE_PORT\nEXPOSE 8080/tcp\nFROM runtime AS final\nENV MODE=prod\nENTRYPOINT [\"/app\"]\nCMD [\"serve\"]\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Extract(path, "container:test")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, s := range f.Environment {
		env[s.Name] = s.Value
	}
	if env["APP_PORT"] != "8080" || env["MODE"] != "prod" {
		t.Fatalf("unexpected env: %v", env)
	}
	if len(f.ExposedPorts) != 1 || f.ExposedPorts[0] != 8080 {
		t.Fatalf("unexpected exposed ports: %v", f.ExposedPorts)
	}
	if len(f.Entrypoint) != 1 || f.Entrypoint[0] != "/app" {
		t.Fatalf("unexpected entrypoint: %v", f.Entrypoint)
	}
}
