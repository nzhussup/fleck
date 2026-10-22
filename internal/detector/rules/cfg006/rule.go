package cfg006

import (
	"context"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/model"
)

type rule struct{}

func New() detector.Detector { return rule{} }

func (rule) Metadata() detector.Metadata {
	return detector.Metadata{
		ID:          "CFG006",
		Name:        "Invalid configuration reference",
		Description: "A reference is proven invalid by resources present in the analyzed manifests",
		Severity:    model.SeverityError,
	}
}

func (rule) Requires() []model.Capability {
	return []model.Capability{model.CapabilityDeploymentEnv}
}

func (current rule) Detect(_ context.Context, analysis *detector.AnalysisContext) ([]model.Finding, error) {
	metadata := current.Metadata()
	findings := make([]model.Finding, 0, len(analysis.Facts.InvalidReferences))
	for _, invalid := range analysis.Facts.InvalidReferences {
		findings = append(findings, model.Finding{
			RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity, Confidence: model.ConfidenceHigh,
			CorrelationKey: "invalid-reference:" + invalid.Kind + ":" + invalid.Reference,
			Subject:        invalid.Subject, Message: invalid.Message, Location: invalid.Location,
			Evidence:   []model.Evidence{{Kind: invalid.Kind, Description: "invalid reference: " + invalid.Reference, Location: invalid.Location}},
			Suggestion: "Correct the reference or provide the referenced mandatory key or port declaration.",
		})
	}
	return findings, nil
}
