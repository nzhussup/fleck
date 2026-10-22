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
	Tool          struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"tool"`
	Target struct {
		Application string `json:"application"`
		Container   string `json:"container"`
		Workload    string `json:"workload"`
	} `json:"target"`
	Findings    []model.Finding    `json:"findings"`
	Diagnostics []model.Diagnostic `json:"diagnostics,omitempty"`
	Summary     Summary            `json:"summary"`
	Timings     map[string]int64   `json:"timingsMs"`
}

func JSON(w io.Writer, version string, facts *model.Facts, findings []model.Finding, t model.Timings) error {
	r := JSONReport{SchemaVersion: "1", Findings: findings, Diagnostics: facts.Diagnostics, Summary: Summarize(findings), Timings: map[string]int64{"goExtraction": t.GoExtraction.Milliseconds(), "helmRendering": t.HelmRendering.Milliseconds(), "kubernetesExtraction": t.KubernetesExtraction.Milliseconds(), "dockerExtraction": t.DockerExtraction.Milliseconds(), "graphConstruction": t.GraphConstruction.Milliseconds(), "detection": t.Detection.Milliseconds(), "total": t.Total.Milliseconds()}}
	if r.Findings == nil {
		r.Findings = []model.Finding{}
	}
	r.Tool.Name = "fleck"
	r.Tool.Version = version
	r.Target.Application = facts.ApplicationID
	r.Target.Container = facts.Container.Name
	r.Target.Workload = facts.Workload.Kind + "/" + facts.Workload.Name
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

func Text(w io.Writer, facts *model.Facts, findings []model.Finding) error {
	if _, err := fmt.Fprintf(w, "Fleck — Configuration Analysis\n\nTarget: %s\nApplication: %s\nDeployment: %s/%s\nContainer: %s\n\n", facts.ApplicationID, facts.ApplicationPath, facts.Workload.Kind, facts.Workload.Name, facts.Container.Name); err != nil {
		return err
	}
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
	s := Summarize(findings)
	_, err := fmt.Fprintf(w, "Summary:\n  Errors:   %d\n  Warnings: %d\n  Info:     %d\n", s.Errors, s.Warnings, s.Info)
	return err
}
