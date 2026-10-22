package detector

import (
	"context"
	"fmt"
	"sort"

	"github.com/nzhussup/fleck/internal/model"
)

func Run(ctx context.Context, detectors []Detector, analysis *AnalysisContext, enabled map[string]bool) ([]model.Finding, error) {
	var findings []model.Finding
	for _, current := range detectors {
		metadata := current.Metadata()
		if len(enabled) > 0 && !enabled[metadata.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		detected, err := current.Detect(ctx, analysis)
		if err != nil {
			return nil, fmt.Errorf("detector %s: %w", metadata.ID, err)
		}
		findings = append(findings, detected...)
	}

	sortFindings(findings)
	return findings, nil
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
