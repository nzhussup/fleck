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
		if len(exposed) > 0 && !setsOverlap(declared, exposed) && !hasVerifiedRuntimeMismatch(analysis) {
			findings = append(findings, model.Finding{
				RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity, Confidence: model.ConfidenceMedium,
				CorrelationKey: "port-metadata:" + analysis.Facts.Container.ID,
				Subject:        analysis.Facts.Container.Name,
				Location:       analysis.Facts.Docker.Location,
				Message: fmt.Sprintf(
					"Dockerfile EXPOSE ports [%s] differ from Kubernetes container ports [%s]",
					formatSet(exposed), formatSet(declared),
				),
				Evidence: []model.Evidence{
					{Kind: "docker-expose", Description: "Dockerfile EXPOSE: " + formatSet(exposed), Location: analysis.Facts.Docker.Location},
					{Kind: "container-port", Description: "Kubernetes containerPort: " + formatSet(declared)},
				},
				Suggestion: "Verify that these metadata declarations describe the port on which the application actually listens.",
			})
		}
	}

	return findings, nil
}

func hasVerifiedRuntimeMismatch(analysis *detector.AnalysisContext) bool {
	if len(analysis.Facts.ListenPorts) != 1 || !analysis.Facts.ListenPorts[0].Resolved {
		return false
	}
	listener := analysis.Facts.ListenPorts[0]
	for _, service := range analysis.Queries.ServicesForWorkload() {
		for _, port := range service.Ports {
			if !port.TargetResolved || port.TargetAmbiguous || !strings.EqualFold(port.Protocol, "TCP") || port.TargetPort == listener.Port {
				continue
			}
			if port.TargetContainer != "" && port.TargetContainer != analysis.Facts.Container.ID {
				continue
			}
			if port.TargetContainer == "" && len(analysis.Facts.Workload.Containers) > 1 {
				continue
			}
			return true
		}
	}
	return false
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

func setsOverlap(left, right map[int]bool) bool {
	for value := range left {
		if right[value] {
			return true
		}
	}
	return false
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
