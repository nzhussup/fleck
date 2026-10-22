package cli

import (
	"bytes"
	"context"
	"errors"
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
	extra := ""
	if r.faulty {
		db = "DATABASES_URL"
		target = "3000"
		extra = "\n        - name: LEGACY_API_KEY\n          value: obsolete-placeholder"
	}
	m := strings.ReplaceAll(strings.ReplaceAll(renderedManifest, "{{DB}}", db), "{{TARGET}}", target)
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

func TestHelmFailureIsAnalysisFailure(t *testing.T) {
	root := NewRoot(Dependencies{Service: &app.Service{Renderer: staticRenderer{fail: true}}, Out: new(bytes.Buffer), Err: new(bytes.Buffer)})
	root.SetArgs([]string{"check", exampleDir(t)})
	err := root.Execute()
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("error=%v", err)
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
        ports: [{name: http, containerPort: 8080, protocol: TCP}]
---
apiVersion: v1
kind: Service
metadata: {name: payment-service}
spec:
  selector: {app: payment-service}
  ports: [{name: http, port: 80, targetPort: {{TARGET}}, protocol: TCP}]
`
