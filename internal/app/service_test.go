package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

func TestAnalyzeSupportsEveryInputLayerCombination(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.test/combinations\n\ngo 1.24\n")
	writeTestFile(t, dir, "main.go", "package main\n\nimport \"os\"\n\nfunc main() { _ = os.Getenv(\"PORT\") }\n")
	writeTestFile(t, dir, "Dockerfile", "FROM scratch\nENV PORT=8080\nEXPOSE 8080\n")
	writeTestFile(t, dir, "k8s/resources.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    metadata: {labels: {app: demo}}
    spec:
      containers:
        - name: app
          env: [{name: PORT, value: "8080"}]
          ports: [{name: http, containerPort: 8080}]
---
apiVersion: v1
kind: Service
metadata: {name: app}
spec:
  selector: {app: demo}
  ports: [{port: 80, targetPort: http}]
`)

	const application = "application: {language: go, path: .}\n"
	const dockerfile = "dockerfile: {path: ./Dockerfile}\n"
	const deployment = `deployment:
  type: kubernetes
  manifests: [./k8s]
  workload: {kind: Deployment, name: app}
  container: app
`
	tests := []struct {
		name                                string
		sections                            string
		application, deployment, dockerfile bool
		wantSkipped                         int
	}{
		{name: "application", sections: application, application: true, wantSkipped: 6},
		{name: "deployment", sections: deployment, deployment: true, wantSkipped: 4},
		{name: "Dockerfile", sections: dockerfile, dockerfile: true, wantSkipped: 6},
		{name: "application and deployment", sections: application + deployment, application: true, deployment: true},
		{name: "application and Dockerfile", sections: application + dockerfile, application: true, dockerfile: true, wantSkipped: 4},
		{name: "deployment and Dockerfile", sections: deployment + dockerfile, deployment: true, dockerfile: true, wantSkipped: 4},
		{name: "all layers", sections: application + deployment + dockerfile, application: true, deployment: true, dockerfile: true},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := filepath.Join(dir, fmt.Sprintf("fleck-%d.yaml", index))
			if err := os.WriteFile(configPath, []byte("version: 1\n"+test.sections), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := New().Analyze(context.Background(), dir, Options{ConfigPath: configPath})
			if err != nil {
				t.Fatal(err)
			}
			if result.Facts.Layers.Application != test.application || result.Facts.Layers.Deployment != test.deployment || result.Facts.Layers.Dockerfile != test.dockerfile {
				t.Fatalf("layers=%#v", result.Facts.Layers)
			}
			if len(result.Skipped) != test.wantSkipped {
				t.Fatalf("skipped=%d, want %d: %#v", len(result.Skipped), test.wantSkipped, result.Skipped)
			}
		})
	}
}

func TestResolveListenPortsUsesEffectiveEnvironmentWithoutUnsafeFallback(t *testing.T) {
	tests := []struct {
		name     string
		supplies []model.EnvSupply
		port     int
		resolved bool
	}{
		{name: "absent uses application fallback", port: 8080, resolved: true},
		{name: "Docker value", supplies: []model.EnvSupply{{Name: "PORT", Value: "3000", ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyDockerfile}}, port: 3000, resolved: true},
		{name: "Kubernetes override", supplies: []model.EnvSupply{{Name: "PORT", Value: "3000", ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyDockerfile}, {Name: "PORT", Value: "9090", ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyKubernetes}}, port: 9090, resolved: true},
		{name: "unverified override blocks Docker and fallback", supplies: []model.EnvSupply{{Name: "PORT", Value: "3000", ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyDockerfile}, {Name: "PORT", State: model.SupplyUnverified, SourceKind: model.SupplyConfigMap}}},
		{name: "invalid override blocks Docker and fallback", supplies: []model.EnvSupply{{Name: "PORT", Value: "3000", ValueKnown: true, State: model.SupplyDefinite, SourceKind: model.SupplyDockerfile}, {Name: "PORT", State: model.SupplyMissingMandatory, SourceKind: model.SupplyConfigMap}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := &model.Facts{ListenPorts: []model.ListenPort{{EnvName: "PORT", DefaultPort: 8080}}, Container: model.Container{Supplies: test.supplies}}
			resolveListenPorts(facts)
			if facts.ListenPorts[0].Port != test.port || facts.ListenPorts[0].Resolved != test.resolved {
				t.Fatalf("listener=%#v", facts.ListenPorts[0])
			}
		})
	}
}

func TestSourcePathIsRelativeToFleckWorkspace(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "workspace")
	path := filepath.Join(root, "application-repo", "build", "Dockerfile.api")
	if got, want := sourcePath(root, path), "application-repo/build/Dockerfile.api"; got != want {
		t.Fatalf("sourcePath()=%q, want %q", got, want)
	}
}

func writeTestFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
