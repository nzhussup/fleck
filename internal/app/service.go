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
)

type Service struct{ Renderer helm.Renderer }
type Options struct {
	ConfigPath    string
	Values        []string
	Rules         []string
	SkipDetection bool
}
type Result struct {
	Config   *config.Config
	Facts    *model.Facts
	Graph    *graph.Graph
	Findings []model.Finding
	Timings  model.Timings
}

func New() *Service { return &Service{Renderer: helm.CommandRenderer{}} }

func (s *Service) Analyze(ctx context.Context, target string, o Options) (Result, error) {
	total := time.Now()
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return Result{}, err
	}
	configPath := o.ConfigPath
	if configPath == "" {
		configPath = filepath.Join(absTarget, "fleck.yaml")
	} else if !filepath.IsAbs(configPath) {
		configPath, _ = filepath.Abs(configPath)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return Result{}, err
	}
	rulesToValidate := cfg.Analysis.Rules
	if o.Rules != nil {
		rulesToValidate = o.Rules
	}
	if err := ValidateRules(rulesToValidate); err != nil {
		return Result{}, fmt.Errorf("invalid analysis rules: %w", err)
	}
	values := cfg.Deployment.Values
	if o.Values != nil {
		values = o.Values
	}
	absValues := make([]string, len(values))
	for i, v := range values {
		absValues[i] = cfg.Resolve(v)
		if o.Values != nil && !filepath.IsAbs(v) {
			absValues[i] = filepath.Join(absTarget, v)
		}
	}
	start := time.Now()
	goFacts, err := goextract.Extract(ctx, cfg.BaseDir, cfg.Resolve(cfg.Application.Path), cfg.Deployment.Workload.Name)
	if err != nil {
		return Result{}, err
	}
	goDuration := time.Since(start)
	renderer := s.Renderer
	if renderer == nil {
		renderer = helm.CommandRenderer{}
	}
	rendered, err := renderer.Render(ctx, cfg.Deployment.Release, cfg.Resolve(cfg.Deployment.Chart), absValues)
	if err != nil {
		return Result{}, err
	}
	start = time.Now()
	kube, err := kubeextract.Extract(rendered.Manifest, cfg.Deployment.Workload.Kind, cfg.Deployment.Workload.Name, cfg.Deployment.Container)
	if err != nil {
		return Result{}, err
	}
	kubeDuration := time.Since(start)
	start = time.Now()
	docker, err := dockerextract.Extract(cfg.Resolve(cfg.Dockerfile.Path), kube.Container.ID)
	if err != nil {
		return Result{}, err
	}
	dockerDuration := time.Since(start)
	kube.Container.Supplies = effectiveOrder(docker.Environment, kube.Container.Supplies)
	facts := &model.Facts{ApplicationID: cfg.Deployment.Workload.Name, ApplicationPath: cfg.Application.Path, EnvReads: goFacts.EnvReads, ListenPorts: goFacts.ListenPorts, Workload: kube.Workload, Container: kube.Container, Services: kube.Services, Docker: docker, Diagnostics: kube.Diagnostics}
	resolveListenPorts(facts)
	start = time.Now()
	g := graph.Build(facts)
	graphDuration := time.Since(start)
	result := Result{Config: cfg, Facts: facts, Graph: g, Timings: model.Timings{GoExtraction: goDuration, HelmRendering: rendered.Duration, KubernetesExtraction: kubeDuration, DockerExtraction: dockerDuration, GraphConstruction: graphDuration}}
	if !o.SkipDetection {
		enabled := map[string]bool{}
		rules := cfg.Analysis.Rules
		if o.Rules != nil {
			rules = o.Rules
		}
		for _, id := range rules {
			enabled[id] = true
		}
		start = time.Now()
		actx := &detector.AnalysisContext{Facts: facts, Graph: g, Queries: graph.Queries{Facts: facts, Graph: g}}
		result.Findings, err = detector.Run(ctx, detectorregistry.All(), actx, enabled)
		result.Timings.Detection = time.Since(start)
		if err != nil {
			return Result{}, err
		}
	}
	result.Timings.Total = time.Since(total)
	return result, nil
}

func effectiveOrder(image, kube []model.EnvSupply) []model.EnvSupply {
	out := append([]model.EnvSupply(nil), image...)
	out = append(out, kube...)
	return out
}
func resolveListenPorts(f *model.Facts) {
	q := graph.Queries{Facts: f}
	for i := range f.ListenPorts {
		p := &f.ListenPorts[i]
		if p.Resolved || p.EnvName == "" {
			continue
		}
		if s, ok := q.FindSupply(p.EnvName); ok {
			if s.ValueKnown {
				n, err := strconv.Atoi(s.Value)
				if err == nil && n > 0 && n <= 65535 {
					p.Port = n
					p.Resolved = true
				}
			}
			continue
		}
		uncertain := q.HasUnverifiedWildcard()
		for _, s := range f.Container.Supplies {
			if s.Name == p.EnvName && s.State == model.SupplyUnverified {
				uncertain = true
			}
		}
		if !uncertain && p.DefaultPort > 0 {
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
