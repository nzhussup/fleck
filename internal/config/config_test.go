package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesRelativePathsAndValidatesEffectivePaths(t *testing.T) {
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
	if err := c.ValidatePaths([]string{c.Resolve(c.Deployment.Values[0])}); err != nil {
		t.Fatal(err)
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

func TestValidateSchema(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "valid", mutate: func(*Config) {}, want: ""},
		{name: "unsupported language", mutate: func(c *Config) { c.Application.Language = "java" }, want: `application.language must be one of [go]`},
		{name: "unsupported deployment", mutate: func(c *Config) { c.Deployment.Type = "raw" }, want: `deployment.type must be one of [helm, kubernetes]`},
		{name: "unsupported workload", mutate: func(c *Config) { c.Deployment.Workload.Kind = "StatefulSet" }, want: `deployment.workload.kind must be one of [Deployment]`},
		{name: "missing fields", mutate: func(c *Config) { c.Application.Path = ""; c.Deployment.Container = "" }, want: "application.path is required"},
		{name: "empty raw manifest path", mutate: func(c *Config) {
			c.Deployment.Type = "kubernetes"
			c.Deployment.Chart = ""
			c.Deployment.Release = ""
			c.Deployment.Manifests = []string{""}
		}, want: "deployment.manifests[0] is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg)
			err := cfg.Validate()
			if test.want == "" && err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error=%v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateAllowsEachLayerIndependently(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "application only", cfg: Config{Version: 1, Application: &ApplicationConfig{Language: "go", Path: "cmd/api"}}},
		{name: "Dockerfile only", cfg: Config{Version: 1, Dockerfile: &DockerfileConfig{Path: "Dockerfile"}}},
		{name: "raw Kubernetes only", cfg: Config{Version: 1, Deployment: &DeploymentConfig{Type: "kubernetes", Manifests: []string{"k8s"}, Workload: WorkloadConfig{Kind: "Deployment", Name: "app"}, Container: "app"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.cfg.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateRejectsConfigWithoutAnInputLayer(t *testing.T) {
	err := (&Config{Version: 1}).Validate()
	if err == nil || !strings.Contains(err.Error(), "at least one target or one of application, deployment, or dockerfile") {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateWorkspaceTargets(t *testing.T) {
	cfg := Config{Version: 1, BaseDir: "/workspace", Repositories: map[string]string{"apps": "repos/apps"}, Targets: []TargetConfig{
		{Name: "api", Application: &ApplicationConfig{Repository: "apps", Language: "go", Path: "api"}, Analysis: AnalysisConfig{Rules: []string{"CFG001", "CFG003"}}},
		{Name: "worker", Dockerfile: &DockerfileConfig{Path: "repos/worker/Dockerfile"}, Analysis: AnalysisConfig{Rules: []string{"CFG005"}}},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.ConfiguredTargets(); len(got) != 2 || got[0].Name != "api" || got[1].Name != "worker" {
		t.Fatalf("targets=%#v", got)
	}
	if got := cfg.ConfiguredTargets(); len(got[0].Analysis.Rules) != 2 || got[0].Analysis.Rules[0] != "CFG001" || len(got[1].Analysis.Rules) != 1 || got[1].Analysis.Rules[0] != "CFG005" {
		t.Fatalf("target rules were not preserved: %#v", got)
	}
	if got := cfg.ResolveIn("apps", "api"); got != filepath.Join("/workspace", "repos/apps", "api") {
		t.Fatalf("resolved repository path=%s", got)
	}
}

func TestValidateWorkspaceRejectsTopLevelRules(t *testing.T) {
	cfg := Config{
		Version:  1,
		Analysis: AnalysisConfig{Rules: []string{"CFG001"}},
		Targets: []TargetConfig{
			{Name: "api", Application: &ApplicationConfig{Language: "go", Path: "cmd/api"}},
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "top-level analysis.rules cannot be combined with targets") {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateWorkspaceRejectsDuplicateNamesAndMixedLegacyFields(t *testing.T) {
	cfg := Config{
		Version:     1,
		Application: &ApplicationConfig{Language: "go", Path: "."},
		Targets: []TargetConfig{
			{Name: "api", Dockerfile: &DockerfileConfig{Path: "Dockerfile"}},
			{Name: "api", Dockerfile: &DockerfileConfig{Path: "Dockerfile"}},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateWorkspaceRejectsUnknownRepositoryAlias(t *testing.T) {
	cfg := Config{Version: 1, Targets: []TargetConfig{{Name: "api", Application: &ApplicationConfig{Repository: "missing", Language: "go", Path: "cmd/api"}}}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `unknown repository "missing"`) {
		t.Fatalf("error=%v", err)
	}
}

func TestValidateRawKubernetesPathsAcceptsFilesAndDirectories(t *testing.T) {
	dir := t.TempDir()
	manifests := filepath.Join(dir, "k8s")
	if err := os.Mkdir(manifests, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "deployment.yaml")
	if err := os.WriteFile(file, []byte("kind: Deployment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Version: 1, BaseDir: dir, Deployment: &DeploymentConfig{Type: "kubernetes", Manifests: []string{"k8s", "deployment.yaml"}, Workload: WorkloadConfig{Kind: "Deployment", Name: "app"}, Container: "app"}}
	if err := cfg.ValidatePaths(nil); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePathsUsesEffectiveValuesOverlays(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "chart"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(dir, "override.yaml")
	if err := os.WriteFile(override, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := validConfig()
	cfg.BaseDir = dir
	cfg.Deployment.Values = []string{"missing-configured-overlay.yaml"}
	if err := cfg.ValidatePaths([]string{override}); err != nil {
		t.Fatalf("valid CLI replacement was rejected: %v", err)
	}
	if err := cfg.ValidatePaths([]string{cfg.Resolve(cfg.Deployment.Values[0])}); err == nil || !strings.Contains(err.Error(), "deployment.values[0]") {
		t.Fatalf("missing effective overlay error=%v", err)
	}
}

func TestValidatePathsReportsAllInvalidInputs(t *testing.T) {
	cfg := validConfig()
	cfg.BaseDir = t.TempDir()
	err := cfg.ValidatePaths([]string{filepath.Join(cfg.BaseDir, "missing-values.yaml")})
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, field := range []string{"application.path", "deployment.chart", "dockerfile.path", "deployment.values[0]"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error does not mention %s: %v", field, err)
		}
	}
}

func validConfig() Config {
	cfg := Config{
		Version:     1,
		Application: &ApplicationConfig{Language: "go", Path: "cmd/api"},
		Deployment:  &DeploymentConfig{Type: "helm", Chart: "chart", Release: "app", Workload: WorkloadConfig{Kind: "Deployment", Name: "app"}, Container: "app"},
		Dockerfile:  &DockerfileConfig{Path: "Dockerfile"},
	}
	return cfg
}
