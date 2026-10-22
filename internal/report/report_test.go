package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nzhussup/fleck/internal/model"
)

func TestJSONReportsAbsentLayersAndSkippedChecksExplicitly(t *testing.T) {
	facts := &model.Facts{
		ApplicationID: "app",
		Layers:        model.LayerAvailability{Deployment: true},
		Workload:      model.Workload{Kind: "Deployment", Name: "app"},
		Container:     model.Container{Name: "app"},
	}
	skipped := []model.SkippedCheck{{RuleID: "CFG001", RuleName: "missing", Reason: "not applicable", Missing: []model.Capability{model.CapabilityEnvReads}}}
	var output bytes.Buffer
	if err := JSON(&output, "test", facts, nil, skipped, model.Timings{}); err != nil {
		t.Fatal(err)
	}
	var report JSONReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Target.Application != "" || !report.Layers.Deployment || report.Layers.Application {
		t.Fatalf("unexpected target/layers: %#v %#v", report.Target, report.Layers)
	}
	if report.Findings == nil || len(report.SkippedChecks) != 1 || report.Summary.Skipped != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestTextDistinguishesAnalyzedAndMissingLayers(t *testing.T) {
	facts := &model.Facts{ApplicationID: "app", Layers: model.LayerAvailability{Dockerfile: true}, Container: model.Container{Name: "image"}}
	var output bytes.Buffer
	if err := Text(&output, facts, nil, nil); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Application source:     not provided") || !strings.Contains(text, "Dockerfile:             analyzed") {
		t.Fatalf("unexpected text report:\n%s", text)
	}
}

func TestWorkspaceJSONContainsNamedTargetsAndAggregateSummary(t *testing.T) {
	inputs := []Input{
		{Name: "api", Facts: &model.Facts{ApplicationID: "api", Layers: model.LayerAvailability{Application: true}}, Findings: []model.Finding{{Severity: model.SeverityWarning}}},
		{Name: "worker", Facts: &model.Facts{ApplicationID: "worker", Layers: model.LayerAvailability{Dockerfile: true}}, Skipped: []model.SkippedCheck{{RuleID: "CFG003"}}},
	}
	var output bytes.Buffer
	if err := JSONWorkspace(&output, "test", inputs); err != nil {
		t.Fatal(err)
	}
	var report WorkspaceJSONReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Targets) != 2 || report.Targets[0].Name != "api" || report.Targets[1].Name != "worker" {
		t.Fatalf("targets=%#v", report.Targets)
	}
	if report.Summary.Warnings != 1 || report.Summary.Skipped != 1 {
		t.Fatalf("summary=%#v", report.Summary)
	}
}
