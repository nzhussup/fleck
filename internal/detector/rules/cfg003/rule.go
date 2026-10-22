package cfg003

import (
	"context"
	"fmt"
	"strings"

	"github.com/nzhussup/fleck/internal/detector"
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
	var findings []model.Finding
	for _, listener := range analysis.Facts.ListenPorts {
		if !listener.Resolved {
			continue
		}
		for _, service := range analysis.Queries.ServicesForWorkload() {
			for _, port := range service.Ports {
				if !port.TargetResolved || !strings.EqualFold(port.Protocol, "TCP") || port.TargetPort == listener.Port {
					continue
				}
				metadata := current.Metadata()
				findings = append(findings, model.Finding{
					RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity,
					Subject:  fmt.Sprint(listener.Port),
					Message:  fmt.Sprintf("Application listens on port %d, but Service %s routes to port %d", listener.Port, service.Name, port.TargetPort),
					Location: listener.Source, Related: []model.SourceLocation{port.Location},
					Evidence:   []model.Evidence{{Kind: "service-target-port", Description: fmt.Sprintf("Service %s targetPort resolves to %d", service.Name, port.TargetPort), Location: port.Location}},
					Suggestion: fmt.Sprintf("Route Service %s to application port %d or change the application listener.", service.Name, listener.Port),
				})
			}
		}
	}
	return findings, nil
}
