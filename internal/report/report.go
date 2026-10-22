package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/nzhussup/fleck/internal/model"
)

type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
	Skipped  int `json:"skipped"`
}

func Summarize(findings []model.Finding) Summary {
	var s Summary
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityError:
			s.Errors++
		case model.SeverityWarning:
			s.Warnings++
		default:
			s.Info++
		}
	}
	return s
}

type JSONReport struct {
	SchemaVersion string `json:"schemaVersion"`
	Tool          Tool   `json:"tool"`
	TargetReport
}

type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type TargetReport struct {
	Name   string `json:"name,omitempty"`
	Target struct {
		Application string `json:"application,omitempty"`
		Container   string `json:"container,omitempty"`
		Workload    string `json:"workload,omitempty"`
	} `json:"target"`
	Layers        model.LayerAvailability `json:"layers"`
	Findings      []model.Finding         `json:"findings"`
	SkippedChecks []model.SkippedCheck    `json:"skippedChecks"`
	Diagnostics   []model.Diagnostic      `json:"diagnostics,omitempty"`
	Summary       Summary                 `json:"summary"`
	Timings       map[string]int64        `json:"timingsMs"`
}

type Input struct {
	Name     string
	Facts    *model.Facts
	Findings []model.Finding
	Skipped  []model.SkippedCheck
	Timings  model.Timings
}

type WorkspaceJSONReport struct {
	SchemaVersion string         `json:"schemaVersion"`
	Tool          Tool           `json:"tool"`
	Targets       []TargetReport `json:"targets"`
	Summary       Summary        `json:"summary"`
}

func JSON(w io.Writer, version string, facts *model.Facts, findings []model.Finding, skipped []model.SkippedCheck, t model.Timings) error {
	r := JSONReport{SchemaVersion: "1", Tool: Tool{Name: "fleck", Version: version}, TargetReport: newTargetReport("", facts, findings, skipped, t)}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

func JSONWorkspace(w io.Writer, version string, inputs []Input) error {
	r := WorkspaceJSONReport{SchemaVersion: "1", Tool: Tool{Name: "fleck", Version: version}, Targets: make([]TargetReport, 0, len(inputs))}
	for _, input := range inputs {
		target := newTargetReport(input.Name, input.Facts, input.Findings, input.Skipped, input.Timings)
		r.Targets = append(r.Targets, target)
		r.Summary.Errors += target.Summary.Errors
		r.Summary.Warnings += target.Summary.Warnings
		r.Summary.Info += target.Summary.Info
		r.Summary.Skipped += target.Summary.Skipped
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

func newTargetReport(name string, facts *model.Facts, findings []model.Finding, skipped []model.SkippedCheck, t model.Timings) TargetReport {
	r := TargetReport{Name: name, Layers: facts.Layers, Findings: findings, SkippedChecks: skipped, Diagnostics: facts.Diagnostics, Summary: Summarize(findings), Timings: map[string]int64{"goExtraction": t.GoExtraction.Milliseconds(), "helmRendering": t.HelmRendering.Milliseconds(), "kubernetesExtraction": t.KubernetesExtraction.Milliseconds(), "dockerExtraction": t.DockerExtraction.Milliseconds(), "graphConstruction": t.GraphConstruction.Milliseconds(), "detection": t.Detection.Milliseconds(), "total": t.Total.Milliseconds()}}
	if r.Findings == nil {
		r.Findings = []model.Finding{}
	}
	if r.SkippedChecks == nil {
		r.SkippedChecks = []model.SkippedCheck{}
	}
	r.Summary.Skipped = len(skipped)
	if facts.Layers.Application {
		r.Target.Application = facts.ApplicationID
	}
	r.Target.Container = facts.Container.Name
	if facts.Workload.Kind != "" || facts.Workload.Name != "" {
		r.Target.Workload = facts.Workload.Kind + "/" + facts.Workload.Name
	}
	return r
}

func TextWorkspace(w io.Writer, inputs []Input) error {
	if _, err := fmt.Fprintf(w, "Fleck — Workspace Configuration Analysis\n\nTargets: %d\n\n", len(inputs)); err != nil {
		return err
	}
	var summary Summary
	for index, input := range inputs {
		if _, err := fmt.Fprintf(w, "===== %s =====\n\n", input.Name); err != nil {
			return err
		}
		if err := Text(w, input.Facts, input.Findings, input.Skipped); err != nil {
			return err
		}
		if index < len(inputs)-1 {
			fmt.Fprintln(w)
		}
		current := Summarize(input.Findings)
		summary.Errors += current.Errors
		summary.Warnings += current.Warnings
		summary.Info += current.Info
		summary.Skipped += len(input.Skipped)
	}
	_, err := fmt.Fprintf(w, "\nWorkspace Summary:\n  Errors:   %d\n  Warnings: %d\n  Info:     %d\n  Skipped:  %d\n", summary.Errors, summary.Warnings, summary.Info, summary.Skipped)
	return err
}

func Text(w io.Writer, facts *model.Facts, findings []model.Finding, skipped []model.SkippedCheck) error {
	if _, err := fmt.Fprintf(w, "Fleck — Configuration Analysis\n\nTarget: %s\n", facts.ApplicationID); err != nil {
		return err
	}
	if facts.Layers.Application {
		fmt.Fprintf(w, "Application: %s\n", facts.ApplicationPath)
	}
	if facts.Layers.Deployment {
		fmt.Fprintf(w, "Deployment: %s/%s\nContainer: %s\n", facts.Workload.Kind, facts.Workload.Name, facts.Container.Name)
	} else if facts.Layers.Dockerfile {
		fmt.Fprintf(w, "Container: %s\n", facts.Container.Name)
	}
	fmt.Fprintln(w, "\nLayers:")
	writeLayer(w, "Application source", facts.Layers.Application)
	writeLayer(w, "Kubernetes deployment", facts.Layers.Deployment)
	writeLayer(w, "Dockerfile", facts.Layers.Dockerfile)
	fmt.Fprintln(w)
	for _, f := range findings {
		fmt.Fprintf(w, "%s %s — %s\n\n  %s\n", f.RuleID, strings.ToUpper(string(f.Severity)), f.RuleName, f.Message)
		if f.Location.File != "" {
			fmt.Fprintf(w, "\n  Location: %s", f.Location.File)
			if f.Location.Line > 0 {
				fmt.Fprintf(w, ":%d:%d", f.Location.Line, f.Location.Column)
			}
			fmt.Fprintln(w)
		}
		for _, e := range f.Evidence {
			fmt.Fprintf(w, "  Evidence: %s\n", e.Description)
		}
		if f.Suggestion != "" {
			fmt.Fprintf(w, "  Suggestion: %s\n", f.Suggestion)
		}
		fmt.Fprintln(w)
	}
	for _, d := range facts.Diagnostics {
		fmt.Fprintf(w, "DIAGNOSTIC %s — %s\n", strings.ToUpper(d.Level), d.Message)
	}
	if len(facts.Diagnostics) > 0 {
		fmt.Fprintln(w)
	}
	for _, check := range skipped {
		fmt.Fprintf(w, "%s SKIPPED — %s\n\n  %s", check.RuleID, check.RuleName, check.Reason)
		if len(check.Missing) > 0 {
			missing := make([]string, len(check.Missing))
			for index, capability := range check.Missing {
				missing[index] = string(capability)
			}
			fmt.Fprintf(w, " (missing: %s)", strings.Join(missing, ", "))
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w)
	}
	s := Summarize(findings)
	_, err := fmt.Fprintf(w, "Summary:\n  Errors:   %d\n  Warnings: %d\n  Info:     %d\n  Skipped:  %d\n", s.Errors, s.Warnings, s.Info, len(skipped))
	return err
}

func writeLayer(w io.Writer, name string, analyzed bool) {
	status := "not provided"
	if analyzed {
		status = "analyzed"
	}
	fmt.Fprintf(w, "  %-23s %s\n", name+":", status)
}
