package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version     int `yaml:"version"`
	Application struct {
		Language string `yaml:"language"`
		Path     string `yaml:"path"`
	} `yaml:"application"`
	Deployment struct {
		Type     string   `yaml:"type"`
		Chart    string   `yaml:"chart"`
		Release  string   `yaml:"release"`
		Values   []string `yaml:"values"`
		Workload struct {
			Kind string `yaml:"kind"`
			Name string `yaml:"name"`
		} `yaml:"workload"`
		Container string `yaml:"container"`
	} `yaml:"deployment"`
	Dockerfile struct {
		Path string `yaml:"path"`
	} `yaml:"dockerfile"`
	Analysis struct{ Rules []string } `yaml:"analysis"`
	BaseDir  string                   `yaml:"-"`
}

func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.BaseDir = filepath.Dir(abs)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Resolve(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(c.BaseDir, filepath.Clean(p))
}

func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("invalid fleck.yaml: version must be 1")
	}
	if c.Application.Language != "go" {
		return fmt.Errorf("invalid fleck.yaml: application.language must be go")
	}
	checks := []struct {
		name, path string
		dir        bool
	}{
		{"application.path", c.Application.Path, true}, {"deployment.chart", c.Deployment.Chart, true}, {"dockerfile.path", c.Dockerfile.Path, false},
	}
	if c.Deployment.Type != "helm" {
		return fmt.Errorf("invalid fleck.yaml: deployment.type must be helm")
	}
	if c.Deployment.Workload.Kind != "Deployment" {
		return fmt.Errorf("invalid fleck.yaml: deployment.workload.kind must be Deployment")
	}
	if c.Deployment.Release == "" || c.Deployment.Workload.Kind == "" || c.Deployment.Workload.Name == "" || c.Deployment.Container == "" {
		return fmt.Errorf("invalid fleck.yaml: release, workload kind/name, and container are required")
	}
	for _, x := range checks {
		if x.path == "" {
			return fmt.Errorf("invalid fleck.yaml: %s is required", x.name)
		}
		st, err := os.Stat(c.Resolve(x.path))
		if err != nil {
			return fmt.Errorf("invalid fleck.yaml: %s %q: %w", x.name, x.path, err)
		}
		if x.dir != st.IsDir() {
			return fmt.Errorf("invalid fleck.yaml: %s has wrong file type", x.name)
		}
	}
	for _, v := range c.Deployment.Values {
		if st, err := os.Stat(c.Resolve(v)); err != nil || st.IsDir() {
			return fmt.Errorf("invalid fleck.yaml: deployment.values %q is not a file", v)
		}
	}
	return nil
}
