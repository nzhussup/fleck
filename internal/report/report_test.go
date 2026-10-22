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

func TestTextShowsRelatedConfigurationLocation(t *testing.T) {
	facts := &model.Facts{ApplicationID: "app", Layers: model.LayerAvailability{Application: true}}
	findings := []model.Finding{{
		RuleID:   "CFG003",
		Severity: model.SeverityError,
		Location: model.SourceLocation{File: "cmd/api/main.go", Line: 20, Column: 4},
		Related:  []model.SourceLocation{{File: "deploy/chart/templates/service.yaml"}},
	}}
	var output bytes.Buffer
	if err := Text(&output, facts, findings, nil); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "Location: cmd/api/main.go:20:4") || !strings.Contains(text, "Related location: deploy/chart/templates/service.yaml") {
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

func TestSummaryGroupsRelatedFindingsWithoutHidingRuleResults(t *testing.T) {
	findings := []model.Finding{
		{RuleID: "CFG001", Severity: model.SeverityError, CorrelationKey: "environment:app:DATABASE_URL"},
		{RuleID: "CFG002", Severity: model.SeverityWarning, CorrelationKey: "environment:app:DATABASE_URL"},
		{RuleID: "CFG004", Severity: model.SeverityInfo, CorrelationKey: "environment:app:LEGACY"},
	}
	summary := Summarize(findings)
	if summary.Errors != 1 || summary.Warnings != 1 || summary.Info != 1 || summary.Issues != 2 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestJSONOutputIsStable(t *testing.T) {
	facts := &model.Facts{ApplicationID: "app", Layers: model.LayerAvailability{Application: true}}
	findings := []model.Finding{{RuleID: "CFG002", RuleName: "name", Severity: model.SeverityWarning, Confidence: model.ConfidenceMedium, Evidence: []model.Evidence{{Kind: "similarity", Attributes: map[string]string{"score": "0.9000", "candidate": "APP_URl"}}}}}
	var first, second bytes.Buffer
	if err := JSON(&first, "test", facts, findings, nil, model.Timings{}); err != nil {
		t.Fatal(err)
	}
	if err := JSON(&second, "test", facts, findings, nil, model.Timings{}); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatalf("JSON output changed between identical runs:\n%s\n%s", first.String(), second.String())
	}
}
