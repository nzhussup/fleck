package cfg005

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/model"
)

type rule struct{}

func New() detector.Detector { return rule{} }

func (rule) Metadata() detector.Metadata {
	return detector.Metadata{
		ID:          "CFG005",
		Name:        "Port declarations differ",
		Description: "Docker and Kubernetes port metadata do not agree",
		Severity:    model.SeverityWarning,
	}
}

func (rule) Requires() []model.Capability {
	return []model.Capability{model.CapabilityContainerPorts}
}

func (current rule) Detect(_ context.Context, analysis *detector.AnalysisContext) ([]model.Finding, error) {
	declared := portSet(analysis.Facts.Container.Ports)
	if len(declared) == 0 {
		return nil, nil
	}

	metadata := current.Metadata()
	var findings []model.Finding
	if analysis.Facts.Capabilities.Has(model.CapabilityDockerPorts) {
		exposed := intSet(analysis.Facts.Docker.ExposedPorts)
		if len(exposed) > 0 && !sameSet(declared, exposed) {
			findings = append(findings, model.Finding{
				RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity,
				Subject: analysis.Facts.Container.Name,
				Message: fmt.Sprintf(
					"Dockerfile EXPOSE ports [%s] differ from Kubernetes container ports [%s]",
					formatSet(exposed), formatSet(declared),
				),
				Evidence: []model.Evidence{
					{Kind: "docker-expose", Description: "Dockerfile EXPOSE: " + formatSet(exposed)},
					{Kind: "container-port", Description: "Kubernetes containerPort: " + formatSet(declared)},
				},
				Suggestion: "Verify that these metadata declarations describe the port on which the application actually listens.",
			})
		}
	}

	for _, service := range analysis.Queries.ServicesForWorkload() {
		for _, port := range service.Ports {
			if !port.TargetResolved || !strings.EqualFold(port.Protocol, "TCP") || declared[port.TargetPort] {
				continue
			}
			findings = append(findings, model.Finding{
				RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity,
				Subject: service.Name,
				Message: fmt.Sprintf(
					"Service %s targets port %d, which is not among Kubernetes container ports [%s]",
					service.Name, port.TargetPort, formatSet(declared),
				),
				Location: port.Location,
				Evidence: []model.Evidence{
					{Kind: "service-target-port", Description: fmt.Sprintf("Service targetPort: %d", port.TargetPort), Location: port.Location},
					{Kind: "container-port", Description: "Kubernetes containerPort: " + formatSet(declared)},
				},
				Suggestion: "Verify the application listener and align port metadata if the difference is unintended.",
			})
		}
	}
	return findings, nil
}

func portSet(ports []model.ContainerPort) map[int]bool {
	values := make([]int, 0, len(ports))
	for _, port := range ports {
		if strings.EqualFold(port.Protocol, "TCP") && port.Port > 0 {
			values = append(values, port.Port)
		}
	}
	return intSet(values)
}

func intSet(values []int) map[int]bool {
	set := make(map[int]bool, len(values))
	for _, value := range values {
		if value > 0 {
			set[value] = true
		}
	}
	return set
}

func sameSet(left, right map[int]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func formatSet(set map[int]bool) string {
	values := make([]int, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Ints(values)
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = fmt.Sprint(value)
	}
	return strings.Join(parts, ", ")
}
