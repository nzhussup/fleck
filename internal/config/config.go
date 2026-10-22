package config

import "path/filepath"

type Config struct {
	Version      int                `yaml:"version"`
	Repositories map[string]string  `yaml:"repositories,omitempty"`
	Targets      []TargetConfig     `yaml:"targets,omitempty"`
	Application  *ApplicationConfig `yaml:"application,omitempty"`
	Deployment   *DeploymentConfig  `yaml:"deployment,omitempty"`
	Dockerfile   *DockerfileConfig  `yaml:"dockerfile,omitempty"`
	Analysis     AnalysisConfig     `yaml:"analysis"`
	BaseDir      string             `yaml:"-"`
}

// TargetConfig describes one application/deployment consistency boundary.
// Paths remain relative to the workspace fleck.yaml, so a target can join
// inputs from multiple checked-out repositories.
type TargetConfig struct {
	Name        string             `yaml:"name"`
	Application *ApplicationConfig `yaml:"application,omitempty"`
	Deployment  *DeploymentConfig  `yaml:"deployment,omitempty"`
	Dockerfile  *DockerfileConfig  `yaml:"dockerfile,omitempty"`
	Analysis    AnalysisConfig     `yaml:"analysis,omitempty"`
}

type ApplicationConfig struct {
	Repository string `yaml:"repository,omitempty"`
	Language   string `yaml:"language"`
	Path       string `yaml:"path"`
}

type DeploymentConfig struct {
	Repository string         `yaml:"repository,omitempty"`
	Type       string         `yaml:"type"`
	Chart      string         `yaml:"chart"`
	Release    string         `yaml:"release"`
	Values     []string       `yaml:"values"`
	Manifests  []string       `yaml:"manifests"`
	Workload   WorkloadConfig `yaml:"workload"`
	Container  string         `yaml:"container"`
}

func (c *Config) HasApplication() bool { return c.Application != nil }
func (c *Config) HasDeployment() bool  { return c.Deployment != nil }
func (c *Config) HasDockerfile() bool  { return c.Dockerfile != nil }

func (t TargetConfig) HasApplication() bool { return t.Application != nil }
func (t TargetConfig) HasDeployment() bool  { return t.Deployment != nil }
func (t TargetConfig) HasDockerfile() bool  { return t.Dockerfile != nil }

func (c *Config) ConfiguredTargets() []TargetConfig {
	if len(c.Targets) > 0 {
		return append([]TargetConfig(nil), c.Targets...)
	}
	name := filepath.Base(c.BaseDir)
	if c.Deployment != nil && c.Deployment.Workload.Name != "" {
		name = c.Deployment.Workload.Name
	}
	return []TargetConfig{{Name: name, Application: c.Application, Deployment: c.Deployment, Dockerfile: c.Dockerfile, Analysis: c.Analysis}}
}

type WorkloadConfig struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type DockerfileConfig struct {
	Repository string `yaml:"repository,omitempty"`
	Path       string `yaml:"path"`
}

type AnalysisConfig struct {
	Rules []string `yaml:"rules"`
}

func (c *Config) Resolve(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(c.BaseDir, filepath.Clean(path))
}

func (c *Config) ResolveIn(repository, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := c.BaseDir
	if repository != "" {
		base = c.Resolve(c.Repositories[repository])
	}
	return filepath.Join(base, filepath.Clean(path))
}
