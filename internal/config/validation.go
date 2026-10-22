package config

import (
	"fmt"
	"os"
	"strings"
)

func (c *Config) Validate() error {
	issues := make([]string, 0)
	if c.Version != 1 {
		issues = append(issues, "version must be 1")
	}
	for name, path := range c.Repositories {
		if strings.TrimSpace(name) == "" {
			issues = append(issues, "repository name cannot be empty")
		}
		if strings.TrimSpace(path) == "" {
			issues = append(issues, fmt.Sprintf("repositories.%s path is required", name))
		}
	}
	hasLegacy := c.HasApplication() || c.HasDeployment() || c.HasDockerfile()
	if len(c.Targets) > 0 && hasLegacy {
		issues = append(issues, "targets cannot be combined with top-level application, deployment, or dockerfile")
	}
	if len(c.Targets) > 0 && len(c.Analysis.Rules) > 0 {
		issues = append(issues, "top-level analysis.rules cannot be combined with targets; configure rules within each target")
	}
	if len(c.Targets) == 0 {
		if !hasLegacy {
			issues = append(issues, "at least one target or one of application, deployment, or dockerfile must be configured")
		} else {
			issues = append(issues, validateTarget(TargetConfig{Application: c.Application, Deployment: c.Deployment, Dockerfile: c.Dockerfile}, "", c.Repositories)...)
		}
	} else {
		seen := map[string]bool{}
		for index, target := range c.Targets {
			prefix := fmt.Sprintf("targets[%d].", index)
			if strings.TrimSpace(target.Name) == "" {
				issues = append(issues, prefix+"name is required")
			} else if seen[target.Name] {
				issues = append(issues, fmt.Sprintf("%sname %q is duplicated", prefix, target.Name))
			}
			seen[target.Name] = true
			issues = append(issues, validateTarget(target, prefix, c.Repositories)...)
		}
	}
	return validationError(issues)
}

func validateTarget(target TargetConfig, prefix string, repositories map[string]string) []string {
	issues := make([]string, 0)
	if !target.HasApplication() && !target.HasDeployment() && !target.HasDockerfile() {
		return append(issues, prefix+"at least one of application, deployment, or dockerfile must be configured")
	}
	required := make([]stringField, 0)
	choices := make([]choiceField, 0)
	if target.HasApplication() {
		issues = append(issues, validateRepositoryReference(prefix+"application.repository", target.Application.Repository, repositories)...)
		required = append(required,
			stringField{name: prefix + "application.language", value: target.Application.Language},
			stringField{name: prefix + "application.path", value: target.Application.Path},
		)
		choices = append(choices, choiceField{name: prefix + "application.language", value: target.Application.Language, allowed: []string{"go"}})
	}
	if target.HasDeployment() {
		issues = append(issues, validateRepositoryReference(prefix+"deployment.repository", target.Deployment.Repository, repositories)...)
		required = append(required,
			stringField{name: prefix + "deployment.type", value: target.Deployment.Type},
			stringField{name: prefix + "deployment.workload.kind", value: target.Deployment.Workload.Kind},
			stringField{name: prefix + "deployment.workload.name", value: target.Deployment.Workload.Name},
			stringField{name: prefix + "deployment.container", value: target.Deployment.Container},
		)
		choices = append(choices,
			choiceField{name: prefix + "deployment.type", value: target.Deployment.Type, allowed: []string{"helm", "kubernetes"}},
			choiceField{name: prefix + "deployment.workload.kind", value: target.Deployment.Workload.Kind, allowed: []string{"Deployment"}},
		)
		switch target.Deployment.Type {
		case "helm":
			required = append(required,
				stringField{name: prefix + "deployment.chart", value: target.Deployment.Chart},
				stringField{name: prefix + "deployment.release", value: target.Deployment.Release},
			)
			if len(target.Deployment.Manifests) > 0 {
				issues = append(issues, prefix+"deployment.manifests is only valid when deployment.type is kubernetes")
			}
		case "kubernetes":
			if len(target.Deployment.Manifests) == 0 {
				issues = append(issues, prefix+"deployment.manifests requires at least one file or directory")
			}
			for index, manifest := range target.Deployment.Manifests {
				required = append(required, stringField{name: fmt.Sprintf("%sdeployment.manifests[%d]", prefix, index), value: manifest})
			}
			if target.Deployment.Chart != "" || target.Deployment.Release != "" || len(target.Deployment.Values) > 0 {
				issues = append(issues, prefix+"deployment.chart, release, and values are only valid when deployment.type is helm")
			}
		}
	}
	if target.HasDockerfile() {
		issues = append(issues, validateRepositoryReference(prefix+"dockerfile.repository", target.Dockerfile.Repository, repositories)...)
		required = append(required, stringField{name: prefix + "dockerfile.path", value: target.Dockerfile.Path})
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			issues = append(issues, field.name+" is required")
		}
	}

	for _, field := range choices {
		if field.value != "" && !contains(field.allowed, field.value) {
			issues = append(issues, fmt.Sprintf("%s must be one of [%s], got %q", field.name, strings.Join(field.allowed, ", "), field.value))
		}
	}

	return issues
}

func validateRepositoryReference(field, repository string, repositories map[string]string) []string {
	if repository == "" {
		return nil
	}
	if _, ok := repositories[repository]; !ok {
		return []string{fmt.Sprintf("%s references unknown repository %q", field, repository)}
	}
	return nil
}

// ValidatePaths validates files used by the effective analysis request. The
// values argument must contain the configured overlays or their CLI replacement.
func (c *Config) ValidatePaths(values []string) error {
	targets := c.ConfiguredTargets()
	if len(targets) != 1 {
		return fmt.Errorf("ValidatePaths requires one configured target")
	}
	return c.ValidateTargetPaths(targets[0], values)
}

func (c *Config) ValidateTargetPaths(target TargetConfig, values []string) error {
	paths := make([]pathField, 0)
	if target.HasApplication() {
		paths = append(paths, pathField{name: "application.path", configured: target.Application.Path, resolved: c.ResolveIn(target.Application.Repository, target.Application.Path), kind: pathDirectory})
	}
	if target.HasDeployment() {
		if target.Deployment.Type == "helm" {
			paths = append(paths, pathField{name: "deployment.chart", configured: target.Deployment.Chart, resolved: c.ResolveIn(target.Deployment.Repository, target.Deployment.Chart), kind: pathDirectory})
			for index, value := range values {
				paths = append(paths, pathField{name: fmt.Sprintf("deployment.values[%d]", index), configured: value, resolved: value, kind: pathFile})
			}
		} else {
			for index, value := range target.Deployment.Manifests {
				paths = append(paths, pathField{name: fmt.Sprintf("deployment.manifests[%d]", index), configured: value, resolved: c.ResolveIn(target.Deployment.Repository, value), kind: pathAny})
			}
		}
	}
	if target.HasDockerfile() {
		paths = append(paths, pathField{name: "dockerfile.path", configured: target.Dockerfile.Path, resolved: c.ResolveIn(target.Dockerfile.Repository, target.Dockerfile.Path), kind: pathFile})
	}

	issues := make([]string, 0)
	for _, path := range paths {
		info, err := os.Stat(path.resolved)
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s %q: %v", path.name, path.configured, err))
			continue
		}
		if path.kind == pathDirectory && !info.IsDir() {
			issues = append(issues, fmt.Sprintf("%s %q must be a directory", path.name, path.configured))
		}
		if path.kind == pathFile && info.IsDir() {
			issues = append(issues, fmt.Sprintf("%s %q must be a file", path.name, path.configured))
		}
	}
	return validationError(issues)
}

type stringField struct{ name, value string }

type choiceField struct {
	name, value string
	allowed     []string
}

type pathKind uint8

const (
	pathFile pathKind = iota
	pathDirectory
	pathAny
)

type pathField struct {
	name, configured, resolved string
	kind                       pathKind
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validationError(issues []string) error {
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("invalid fleck.yaml:\n  - %s", strings.Join(issues, "\n  - "))
}
