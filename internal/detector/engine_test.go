package detector

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

type stubDetector struct {
	metadata Metadata
	findings []model.Finding
}

func (stub stubDetector) Metadata() Metadata { return stub.metadata }
func (stub stubDetector) Detect(context.Context, *AnalysisContext) ([]model.Finding, error) {
	return stub.findings, nil
}

func TestRunSelectsRulesAndSortsFindings(t *testing.T) {
	detectors := []Detector{
		stubDetector{metadata: Metadata{ID: "B"}, findings: []model.Finding{{RuleID: "B", Subject: "z"}}},
		stubDetector{metadata: Metadata{ID: "A"}, findings: []model.Finding{{RuleID: "A", Subject: "x"}}},
	}
	findings, err := Run(context.Background(), detectors, &AnalysisContext{}, map[string]bool{"A": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].RuleID != "A" {
		t.Fatalf("unexpected findings: %#v", findings)
	}
}

func TestRunHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, []Detector{stubDetector{metadata: Metadata{ID: "A"}}}, &AnalysisContext{}, nil)
	if err != context.Canceled {
		t.Fatalf("error=%v", err)
	}
}
