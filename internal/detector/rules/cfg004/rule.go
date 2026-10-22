package cfg004

import (
	"context"
	"fmt"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/model"
)

type rule struct{}

func New() detector.Detector { return rule{} }

func (rule) Metadata() detector.Metadata {
	return detector.Metadata{ID: "CFG004", Name: "Potentially unused environment variable", Description: "A supplied variable has no recognized application read", Severity: model.SeverityInfo}
}

func (current rule) Detect(_ context.Context, analysis *detector.AnalysisContext) ([]model.Finding, error) {
	reads := map[string]bool{}
	for _, read := range analysis.Queries.EnvironmentReads() {
		if !read.Resolved {
			return nil, nil
		}
		reads[read.Name] = true
	}

	systemVariables := map[string]bool{"PATH": true, "HOME": true, "HOSTNAME": true, "TERM": true}
	seen := map[string]bool{}
	var findings []model.Finding
	supplies := analysis.Queries.EffectiveSupplies()
	for index := len(supplies) - 1; index >= 0; index-- {
		supply := supplies[index]
		if supply.State != model.SupplyDefinite || supply.Name == "" || reads[supply.Name] || systemVariables[supply.Name] || seen[supply.Name] {
			continue
		}
		seen[supply.Name] = true

		metadata := current.Metadata()
		findings = append(findings, model.Finding{
			RuleID: metadata.ID, RuleName: metadata.Name, Severity: metadata.Severity,
			Subject:    supply.Name,
			Message:    fmt.Sprintf("%s is supplied to container %s, but no matching environment-variable read was found", supply.Name, analysis.Facts.Container.Name),
			Location:   supply.Location,
			Evidence:   []model.Evidence{{Kind: "unmatched-supply", Description: "no recognized os.Getenv or os.LookupEnv read matched this name", Location: supply.Location}},
			Suggestion: "Verify whether an entrypoint, dependency, or runtime component consumes this variable before removing it.",
		})
	}
	return findings, nil
}
