package cfg003

import (
	"context"
	"fmt"
	"strings"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

type rule struct{}

func New() detector.Detector { return rule{} }

func (rule) Metadata() detector.Metadata {
	return detector.Metadata{ID: "CFG003", Name: "Port configuration mismatch", Description: "A Service routes to a different port than the application listens on", Severity: model.SeverityError}
}

func (rule) Requires() []model.Capability {
	return []model.Capability{model.CapabilityAppListenPorts, model.CapabilityServicePorts}
}

func (current rule) Detect(_ context.Context, analysis *detector.AnalysisContext) ([]model.Finding, error) {
	// Without a handler-to-Service mapping, multiple listeners make ownership
	// ambiguous. A definitive mismatch is justified only for one known listener.
	if len(analysis.Facts.ListenPorts) != 1 || !analysis.Facts.ListenPorts[0].Resolved {
		return nil, nil
	}
	listener := analysis.Facts.ListenPorts[0]
	var findings []model.Finding
	for _, service := range analysis.Queries.ServicesForWorkload() {
		for _, port := range service.Ports {
			if !port.TargetResolved || port.TargetAmbiguous || !strings.EqualFold(port.Protocol, "TCP") || port.TargetPort == listener.Port {
				continue
			}
			if port.TargetContainer != "" && port.TargetContainer != analysis.Facts.Container.ID {
				continue
			}
			if port.TargetContainer == "" && len(analysis.Facts.Workload.Containers) > 1 {
				// Numeric target ports address the Pod network namespace. With
				// multiple containers, another process may own that destination.
				continue
			}
			metadata := current.Metadata()
			path := evidencePath(analysis.Graph, service.ID, listener.ID)
			findings = append(findings, model.Finding{
				RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity, Confidence: model.ConfidenceHigh,
				CorrelationKey: "service-port:" + service.ID + ":" + fmt.Sprint(port.Port),
				Subject:        fmt.Sprint(listener.Port),
				Message:        fmt.Sprintf("Application listens on port %d, but Service %s routes to port %d", listener.Port, service.Name, port.TargetPort),
				Location:       listener.Source, Related: []model.SourceLocation{port.Location},
				Evidence:   []model.Evidence{{Kind: "service-target-port", Description: fmt.Sprintf("Service %s targetPort resolves to %d", service.Name, port.TargetPort), Location: port.Location, Path: path}},
				Suggestion: fmt.Sprintf("Route Service %s to application port %d or change the application listener.", service.Name, listener.Port),
			})
		}
	}
	return findings, nil
}

func evidencePath(g *graph.Graph, from, to string) []string {
	path := g.EvidencePath(graph.NodeID(from), graph.NodeID(to))
	result := make([]string, len(path))
	for index, node := range path {
		result[index] = string(node)
	}
	return result
}
