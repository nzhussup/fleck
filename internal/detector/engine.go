package detector

import (
	"context"
	"fmt"
	"sort"

	"github.com/nzhussup/fleck/internal/model"
)

func Run(ctx context.Context, detectors []Detector, analysis *AnalysisContext, enabled map[string]bool) ([]model.Finding, []model.SkippedCheck, error) {
	var findings []model.Finding
	var skipped []model.SkippedCheck
	for _, current := range detectors {
		metadata := current.Metadata()
		if len(enabled) > 0 && !enabled[metadata.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		missing := missingCapabilities(current.Requires(), analysis.Facts.Capabilities)
		if len(missing) > 0 {
			skipped = append(skipped, model.SkippedCheck{RuleID: metadata.ID, RuleName: metadata.Name, Reason: "required analysis capabilities were not available", Missing: missing})
			continue
		}

		detected, err := current.Detect(ctx, analysis)
		if err != nil {
			return nil, nil, fmt.Errorf("detector %s: %w", metadata.ID, err)
		}
		findings = append(findings, detected...)
	}

	sortFindings(findings)
	return findings, skipped, nil
}

func missingCapabilities(required []model.Capability, available model.CapabilitySet) []model.Capability {
	var missing []model.Capability
	for _, capability := range required {
		if !available.Has(capability) {
			missing = append(missing, capability)
		}
	}
	return missing
}

func sortFindings(findings []model.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].RuleID != findings[j].RuleID {
			return findings[i].RuleID < findings[j].RuleID
		}
		if findings[i].Location.File != findings[j].Location.File {
			return findings[i].Location.File < findings[j].Location.File
		}
		if findings[i].Location.Line != findings[j].Location.Line {
			return findings[i].Location.Line < findings[j].Location.Line
		}
		return findings[i].Subject < findings[j].Subject
	})
}
