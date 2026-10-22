package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nzhussup/fleck/internal/app"
	"github.com/nzhussup/fleck/internal/extractor/helm"
)

type staticRenderer struct {
	faulty bool
	fail   bool
}

func (r staticRenderer) Render(_ context.Context, _, _ string, _ []string) (helm.Result, error) {
	if r.fail {
		return helm.Result{}, errors.New("render failed")
	}
	db := "DATABASE_URL"
	target := "http"
	containerPort := "8080"
	extra := ""
	if r.faulty {
		db = "DATABASES_URL"
		containerPort = "3000"
		extra = "\n        - name: LEGACY_API_KEY\n          value: obsolete-placeholder"
	}
	m := strings.ReplaceAll(strings.ReplaceAll(renderedManifest, "{{DB}}", db), "{{TARGET}}", target)
	m = strings.ReplaceAll(m, "{{CONTAINER_PORT}}", containerPort)
	m = strings.ReplaceAll(m, "{{EXTRA}}", extra)
	return helm.Result{Manifest: []byte(m), Duration: time.Millisecond}, nil
}

func TestCheckJSONAndRuleSelection(t *testing.T) {
	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{}}, Out: out, Err: out})
	root.SetArgs([]string{"check", exampleDir(t), "--format", "json", "--rules", "CFG001"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"schemaVersion": "1"`) || !strings.Contains(got, `"findings": []`) {
		t.Fatalf("unexpected JSON: %s", got)
	}
	if strings.Contains(got, "development-token") {
		t.Fatal("secret value leaked")
	}
}

func TestFaultyFixtureProducesAllRulesAndExitOne(t *testing.T) {
	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{faulty: true}}, Out: out, Err: out})
	root.SetArgs([]string{"check", exampleDir(t), "--format", "text"})
	err := root.Execute()
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("exit error=%v", err)
	}
	for _, id := range []string{"CFG001", "CFG002", "CFG003", "CFG004"} {
		if !strings.Contains(out.String(), id) {
			t.Errorf("output missing %s:\n%s", id, out.String())
		}
	}
}

func TestGraphAndRulesCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{{[]string{"graph", exampleDir(t)}, "digraph fleck"}, {[]string{"rules", "list"}, "CFG004"}, {[]string{"version"}, "fleck "}} {
		out := new(bytes.Buffer)
		root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{}}, Out: out, Err: out})
		root.SetArgs(tc.args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("%v output=%s", tc.args, out.String())
		}
	}
}

func TestWorkspaceCheckAndTargetSelection(t *testing.T) {
	workspace := repositoryExample(t, "microservices-monorepo")
	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: app.New(), Out: out, Err: out})
	root.SetArgs([]string{"check", workspace, "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"targets": [`) || !strings.Contains(got, `"name": "orders"`) || !strings.Contains(got, `"name": "notifications"`) {
		t.Fatalf("unexpected workspace report: %s", got)
	}

	out.Reset()
	root = NewRoot(Dependencies{Service: app.New(), Out: out, Err: out})
	root.SetArgs([]string{"graph", workspace, "--target", "orders"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "digraph fleck") {
		t.Fatalf("unexpected graph: %s", out.String())
	}
}

func TestHelmFailureIsAnalysisFailure(t *testing.T) {
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{fail: true}}, Out: new(bytes.Buffer), Err: new(bytes.Buffer)})
	root.SetArgs([]string{"check", exampleDir(t)})
	err := root.Execute()
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("error=%v", err)
	}
}

func TestCheckApplicationOnlyReportsUnavailableCrossLayerChecks(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go.mod", "module example.test/app\n\ngo 1.24\n")
	writeFixture(t, dir, "main.go", "package main\n\nimport \"os\"\n\nfunc main() { _ = os.Getenv(\"PORT\") }\n")
	writeFixture(t, dir, "fleck.yaml", "version: 1\napplication: {language: go, path: .}\n")

	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{fail: true}}, Out: out, Err: out})
	root.SetArgs([]string{"check", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Application source:     analyzed") || !strings.Contains(got, "Kubernetes deployment:  not provided") {
		t.Fatalf("layer availability missing:\n%s", got)
	}
	for _, id := range []string{"CFG001", "CFG002", "CFG003", "CFG004", "CFG005", "CFG006"} {
		if !strings.Contains(got, id+" SKIPPED") {
			t.Errorf("output missing skipped %s:\n%s", id, got)
		}
	}
}

func TestCheckRawKubernetesWithoutApplicationOrDockerfile(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "fleck.yaml", `version: 1
deployment:
  type: kubernetes
  manifests: [./k8s]
  workload: {kind: Deployment, name: app}
  container: app
`)
	writeFixture(t, dir, "k8s/resources.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    metadata: {labels: {app: demo}}
    spec:
      containers:
        - name: app
          ports: [{name: http, containerPort: 8080}]
---
apiVersion: v1
kind: Service
metadata: {name: app}
spec:
  selector: {app: demo}
  ports: [{port: 80, targetPort: http}]
`)

	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{fail: true}}, Out: out, Err: out})
	root.SetArgs([]string{"check", dir, "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"application": false`) || !strings.Contains(got, `"deployment": true`) || !strings.Contains(got, `"dockerfile": false`) {
		t.Fatalf("unexpected layer report: %s", got)
	}
	if !strings.Contains(got, `"ruleId": "CFG001"`) || strings.Contains(got, `"ruleId": "CFG005"`) {
		t.Fatalf("unexpected skipped checks: %s", got)
	}
}

func TestCheckDockerfileOnly(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "fleck.yaml", "version: 1\ndockerfile: {path: ./Dockerfile}\n")
	writeFixture(t, dir, "Dockerfile", "FROM scratch\nENV PORT=8080\nEXPOSE 8080\n")

	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{fail: true}}, Out: out, Err: out})
	root.SetArgs([]string{"check", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Dockerfile:             analyzed") || !strings.Contains(out.String(), "Skipped:  6") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestConfirmedInvalidReferenceReturnsExitOne(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "fleck.yaml", `version: 1
deployment:
  type: kubernetes
  manifests: [./resources.yaml]
  workload: {kind: Deployment, name: app}
  container: app
analysis: {rules: [CFG006]}
`)
	writeFixture(t, dir, "resources.yaml", `apiVersion: v1
kind: ConfigMap
metadata: {name: settings}
data: {present: value}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    spec:
      containers:
        - name: app
          env:
            - name: REQUIRED
              valueFrom: {configMapKeyRef: {name: settings, key: missing}}
`)

	out := new(bytes.Buffer)
	root := NewRoot(Dependencies{Service: app.New(), Out: out, Err: out})
	root.SetArgs([]string{"check", dir})
	err := root.Execute()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("error=%v output=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "CFG006") || strings.Contains(out.String(), "Diagnostics:") {
		t.Fatalf("invalid reference was not reported as a finding:\n%s", out.String())
	}
}

func writeFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exampleDir(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "examples", "payment-service"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func repositoryExample(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

const renderedManifest = `apiVersion: v1
kind: ConfigMap
metadata: {name: payment-service-config}
data:
  database-url: postgres://example.invalid/payments
  app-port: "8080"
  enable-metrics: "false"
---
apiVersion: v1
kind: Secret
metadata: {name: payment-service-secrets}
stringData:
  api-token: development-token
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: payment-service}
spec:
  template:
    metadata: {labels: {app: payment-service}}
    spec:
      containers:
      - name: sidecar
        env: [{name: DATABASE_URL, value: sidecar-only}]
      - name: payment-service
        env:
        - name: {{DB}}
          valueFrom: {configMapKeyRef: {name: payment-service-config, key: database-url}}
        - name: APP_PORT
          valueFrom: {configMapKeyRef: {name: payment-service-config, key: app-port}}
        - name: ENABLE_METRICS
          valueFrom: {configMapKeyRef: {name: payment-service-config, key: enable-metrics}}
        - name: API_TOKEN
          valueFrom: {secretKeyRef: {name: payment-service-secrets, key: api-token}}{{EXTRA}}
        ports: [{name: http, containerPort: {{CONTAINER_PORT}}, protocol: TCP}]
---
apiVersion: v1
kind: Service
metadata: {name: payment-service}
spec:
  selector: {app: payment-service}
  ports: [{name: http, port: 80, targetPort: {{TARGET}}, protocol: TCP}]
`
