package cfg001

import (
	"context"
	"fmt"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/model"
	"github.com/nzhussup/fleck/internal/resolver"
)

type rule struct{}

func New() detector.Detector { return rule{} }

func (rule) Metadata() detector.Metadata {
	return detector.Metadata{ID: "CFG001", Name: "Missing required environment variable", Description: "Required variable is definitely not supplied to the selected container", Severity: model.SeverityError}
}

func (rule) Requires() []model.Capability {
	return []model.Capability{model.CapabilityEnvReads, model.CapabilityDeploymentEnv}
}

func (current rule) Detect(_ context.Context, analysis *detector.AnalysisContext) ([]model.Finding, error) {
	var findings []model.Finding
	for _, read := range analysis.Queries.EnvironmentReads() {
		if !read.Resolved || read.Requirement != model.RequirementRequired {
			continue
		}
		resolution := analysis.Queries.ResolveEnvironment(read.Name)
		if resolution.Status != resolver.EnvironmentAbsent {
			continue
		}

		metadata := current.Metadata()
		findings = append(findings, model.Finding{
			RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity, Confidence: model.ConfidenceHigh,
			CorrelationKey: "environment:" + analysis.Facts.ApplicationID + ":" + read.Name,
			Subject:        read.Name,
			Message:        fmt.Sprintf("Required environment variable %s is not supplied to container %s", read.Name, analysis.Facts.Container.Name),
			Location:       read.Source, Evidence: read.Evidence,
			Suggestion: "Supply " + read.Name + " to the selected container or add an explicit supported application fallback.",
		})
	}
	return findings, nil
}
