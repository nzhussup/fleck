package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/nzhussup/fleck/internal/config"
	"github.com/nzhussup/fleck/internal/detector"
	detectorregistry "github.com/nzhussup/fleck/internal/detector/registry"
	dockerextract "github.com/nzhussup/fleck/internal/extractor/dockerfile"
	goextract "github.com/nzhussup/fleck/internal/extractor/golang"
	"github.com/nzhussup/fleck/internal/extractor/helm"
	kubeextract "github.com/nzhussup/fleck/internal/extractor/kubernetes"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
	"github.com/nzhussup/fleck/internal/resolver"
)

type Service struct{ Renderer helm.Renderer }
type Options struct {
	ConfigPath    string
	Target        string
	Values        []string
	Rules         []string
	SkipDetection bool
}
type Result struct {
	Name     string
	Config   *config.Config
	Facts    *model.Facts
	Graph    *graph.Graph
	Findings []model.Finding
	Skipped  []model.SkippedCheck
	Timings  model.Timings
}

func New() *Service { return &Service{Renderer: helm.CommandRenderer{}} }

func (s *Service) Analyze(ctx context.Context, target string, o Options) (Result, error) {
	results, err := s.AnalyzeAll(ctx, target, o)
	if err != nil {
		return Result{}, err
	}
	if len(results) != 1 {
		return Result{}, fmt.Errorf("configuration contains %d targets; select one with --target", len(results))
	}
	return results[0], nil
}

func (s *Service) AnalyzeAll(ctx context.Context, repository string, o Options) ([]Result, error) {
	absTarget, err := filepath.Abs(repository)
	if err != nil {
		return nil, err
	}
	configPath := o.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(absTarget, "fleck.yaml")
	} else if !filepath.IsAbs(configPath) {
		configPath, _ = filepath.Abs(configPath)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	targets := cfg.ConfiguredTargets()
	if o.Target != "" {
		selected := targets[:0]
		for _, configured := range targets {
			if configured.Name == o.Target {
				selected = append(selected, configured)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("target %q is not configured", o.Target)
		}
		targets = selected
	}
	if o.Values != nil && len(targets) != 1 {
		return nil, fmt.Errorf("--values requires selecting exactly one target with --target")
	}
	results := make([]Result, 0, len(targets))
	for _, configured := range targets {
		result, analyzeErr := s.analyzeTarget(ctx, absTarget, cfg, configured, o)
		if analyzeErr != nil {
			return nil, fmt.Errorf("target %s: %w", configured.Name, analyzeErr)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) analyzeTarget(ctx context.Context, repository string, cfg *config.Config, target config.TargetConfig, o Options) (Result, error) {
	total := time.Now()
	rules := target.Analysis.Rules
	if o.Rules != nil {
		rules = o.Rules
	}
	if err := ValidateRules(rules); err != nil {
		return Result{}, fmt.Errorf("invalid analysis rules: %w", err)
	}
	absValues, err := effectiveValues(cfg, target, repository, o.Values)
	if err != nil {
		return Result{}, err
	}
	if err := cfg.ValidateTargetPaths(target, absValues); err != nil {
		return Result{}, err
	}
	facts := &model.Facts{
		ApplicationID: target.Name,
		Capabilities:  model.CapabilitySet{},
		Layers:        model.LayerAvailability{Application: target.HasApplication(), Deployment: target.HasDeployment(), Dockerfile: target.HasDockerfile()},
	}
	timings := model.Timings{}

	if target.HasApplication() {
		facts.ApplicationPath = target.Application.Path
		start := time.Now()
		goFacts, extractErr := goextract.Extract(ctx, cfg.BaseDir, cfg.ResolveIn(target.Application.Repository, target.Application.Path), facts.ApplicationID)
		timings.GoExtraction = time.Since(start)
		if extractErr != nil {
			return Result{}, extractErr
		}
		facts.EnvReads, facts.ListenPorts = goFacts.EnvReads, goFacts.ListenPorts
		facts.Capabilities[model.CapabilityEnvReads] = true
		facts.Capabilities[model.CapabilityAppListenPorts] = true
	}

	if target.HasDeployment() {
		manifest, renderDuration, renderErr := s.deploymentManifest(ctx, cfg, target, absValues)
		timings.HelmRendering = renderDuration
		if renderErr != nil {
			return Result{}, renderErr
		}
		start := time.Now()
		origins := kubeextract.OriginOptions{RootPath: cfg.BaseDir}
		if target.Deployment.Type == "helm" {
			origins.ChartPath = cfg.ResolveIn(target.Deployment.Repository, target.Deployment.Chart)
		}
		kube, extractErr := kubeextract.ExtractInNamespaceWithOrigins(manifest, target.Deployment.Workload.Kind, target.Deployment.Workload.Name, target.Deployment.Workload.Namespace, target.Deployment.Container, origins)
		timings.KubernetesExtraction = time.Since(start)
		if extractErr != nil {
			return Result{}, extractErr
		}
		facts.Workload, facts.Container, facts.Services, facts.Diagnostics, facts.InvalidReferences = kube.Workload, kube.Container, kube.Services, kube.Diagnostics, kube.InvalidReferences
		facts.Capabilities[model.CapabilityDeploymentEnv] = true
		facts.Capabilities[model.CapabilityEnvSupplies] = true
		facts.Capabilities[model.CapabilityContainerPorts] = true
		facts.Capabilities[model.CapabilityServicePorts] = true
	}

	if target.HasDockerfile() {
		containerID := facts.Container.ID
		if containerID == "" {
			containerID = "container:image"
			facts.Container = model.Container{ID: containerID, Name: "image"}
		}
		start := time.Now()
		dockerfilePath := cfg.ResolveIn(target.Dockerfile.Repository, target.Dockerfile.Path)
		docker, extractErr := dockerextract.ExtractWithSource(dockerfilePath, sourcePath(cfg.BaseDir, dockerfilePath), containerID)
		timings.DockerExtraction = time.Since(start)
		if extractErr != nil {
			return Result{}, extractErr
		}
		facts.Docker = docker
		facts.Container.Supplies = effectiveOrder(docker.Environment, facts.Container.Supplies)
		facts.Capabilities[model.CapabilityDockerEnv] = true
		facts.Capabilities[model.CapabilityEnvSupplies] = true
		facts.Capabilities[model.CapabilityDockerPorts] = true
	}

	resolveListenPorts(facts)
	start := time.Now()
	g := graph.Build(facts)
	timings.GraphConstruction = time.Since(start)
	result := Result{Name: target.Name, Config: cfg, Facts: facts, Graph: g, Timings: timings}
	if !o.SkipDetection {
		enabled := map[string]bool{}
		for _, id := range rules {
			enabled[id] = true
		}
		start = time.Now()
		actx := &detector.AnalysisContext{Facts: facts, Graph: g, Queries: graph.Queries{Facts: facts, Graph: g}}
		result.Findings, result.Skipped, err = detector.Run(ctx, detectorregistry.All(), actx, enabled)
		result.Timings.Detection = time.Since(start)
		if err != nil {
			return Result{}, err
		}
	}
	result.Timings.Total = time.Since(total)
	return result, nil
}

func sourcePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(relative)
}

func effectiveValues(cfg *config.Config, configured config.TargetConfig, repository string, overrides []string) ([]string, error) {
	if !configured.HasDeployment() || configured.Deployment.Type != "helm" {
		if overrides != nil {
			return nil, fmt.Errorf("--values requires a Helm deployment")
		}
		return nil, nil
	}
	values := configured.Deployment.Values
	if overrides != nil {
		values = overrides
	}
	resolved := make([]string, len(values))
	for index, value := range values {
		resolved[index] = cfg.ResolveIn(configured.Deployment.Repository, value)
		if overrides != nil && !filepath.IsAbs(value) {
			resolved[index] = filepath.Join(repository, value)
		}
	}
	return resolved, nil
}

func (s *Service) deploymentManifest(ctx context.Context, cfg *config.Config, target config.TargetConfig, values []string) ([]byte, time.Duration, error) {
	if target.Deployment.Type == "kubernetes" {
		paths := make([]string, len(target.Deployment.Manifests))
		for index, path := range target.Deployment.Manifests {
			paths[index] = cfg.ResolveIn(target.Deployment.Repository, path)
		}
		manifest, err := kubeextract.LoadManifests(ctx, paths)
		return manifest, 0, err
	}
	renderer := s.Renderer
	if renderer == nil {
		renderer = helm.CommandRenderer{}
	}
	rendered, err := renderer.Render(ctx, target.Deployment.Release, cfg.ResolveIn(target.Deployment.Repository, target.Deployment.Chart), values)
	return rendered.Manifest, rendered.Duration, err
}

func effectiveOrder(image, kube []model.EnvSupply) []model.EnvSupply {
	out := append([]model.EnvSupply(nil), image...)
	out = append(out, kube...)
	return out
}
func resolveListenPorts(f *model.Facts) {
	for i := range f.ListenPorts {
		p := &f.ListenPorts[i]
		if p.Resolved || p.EnvName == "" {
			continue
		}
		resolved := resolver.Resolve(f.Container.Supplies, p.EnvName)
		if resolved.Status == resolver.EnvironmentResolved {
			if resolved.ValueKnown {
				n, err := strconv.Atoi(resolved.Value)
				if err == nil && n > 0 && n <= 65535 {
					p.Port = n
					p.Resolved = true
				}
			}
			continue
		}
		if resolved.Status == resolver.EnvironmentAbsent && p.DefaultPort > 0 {
			p.Port = p.DefaultPort
			p.Resolved = true
		}
	}
}

func ValidateRules(ids []string) error {
	known := map[string]bool{}
	for _, m := range detectorregistry.Metadata() {
		known[m.ID] = true
	}
	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("unknown rule %q", id)
		}
	}
	return nil
}
func ParseRules(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	parts := splitComma(s)
	if err := ValidateRules(parts); err != nil {
		return nil, err
	}
	return parts, nil
}
func splitComma(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range stringsSplit(s, ',') {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
func stringsSplit(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == sep {
			x := s[start:i]
			for len(x) > 0 && (x[0] == ' ' || x[0] == '\t') {
				x = x[1:]
			}
			for len(x) > 0 && (x[len(x)-1] == ' ' || x[len(x)-1] == '\t') {
				x = x[:len(x)-1]
			}
			out = append(out, x)
			start = i + 1
		}
	}
	return out
}
