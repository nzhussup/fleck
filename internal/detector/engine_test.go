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

func (stub stubDetector) Metadata() Metadata           { return stub.metadata }
func (stub stubDetector) Requires() []model.Capability { return nil }
func (stub stubDetector) Detect(context.Context, *AnalysisContext) ([]model.Finding, error) {
	return stub.findings, nil
}

func TestRunSelectsRulesAndSortsFindings(t *testing.T) {
	detectors := []Detector{
		stubDetector{metadata: Metadata{ID: "B"}, findings: []model.Finding{{RuleID: "B", Subject: "z"}}},
		stubDetector{metadata: Metadata{ID: "A"}, findings: []model.Finding{{RuleID: "A", Subject: "x"}}},
	}
	findings, skipped, err := Run(context.Background(), detectors, &AnalysisContext{Facts: &model.Facts{Capabilities: model.CapabilitySet{}}}, map[string]bool{"A": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Fatalf("unexpected skipped checks: %#v", skipped)
	}
	if len(findings) != 1 || findings[0].RuleID != "A" {
		t.Fatalf("unexpected findings: %#v", findings)
	}
}

func TestRunHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := Run(ctx, []Detector{stubDetector{metadata: Metadata{ID: "A"}}}, &AnalysisContext{Facts: &model.Facts{Capabilities: model.CapabilitySet{}}}, nil)
	if err != context.Canceled {
		t.Fatalf("error=%v", err)
	}
}

func TestRunReportsMissingCapabilities(t *testing.T) {
	rule := capabilityStub{stubDetector: stubDetector{metadata: Metadata{ID: "A", Name: "needs source"}}}
	findings, skipped, err := Run(context.Background(), []Detector{rule}, &AnalysisContext{Facts: &model.Facts{Capabilities: model.CapabilitySet{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 || len(skipped) != 1 || skipped[0].Missing[0] != model.CapabilityEnvReads {
		t.Fatalf("unexpected result: findings=%#v skipped=%#v", findings, skipped)
	}
}

type capabilityStub struct{ stubDetector }

func (capabilityStub) Requires() []model.Capability {
	return []model.Capability{model.CapabilityEnvReads}
}
