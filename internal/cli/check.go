package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/nzhussup/fleck/internal/app"
	"github.com/nzhussup/fleck/internal/model"
	"github.com/nzhussup/fleck/internal/report"
	"github.com/spf13/cobra"
)

func newCheck(d Dependencies) *cobra.Command {
	var configPath, targetName, format, output, rules, severity, failOn string
	var values []string
	cmd := &cobra.Command{Use: "check [repository]", Short: "Analyze a configured project", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		target := "."
		if len(args) == 1 {
			target = args[0]
		}
		selected, err := app.ParseRules(rules)
		if err != nil {
			return err
		}
		if err = validFormat(format, "text", "json"); err != nil {
			return err
		}
		min, err := parseSeverity(severity)
		if err != nil {
			return err
		}
		fail, err := parseSeverity(failOn)
		if err != nil {
			return err
		}
		var overrides []string
		if cmd.Flags().Changed("values") {
			overrides = values
		}
		results, err := d.Service.AnalyzeAll(cmd.Context(), target, app.Options{ConfigPath: configPath, Target: targetName, Values: overrides, Rules: selected})
		if err != nil {
			return &ExitError{Code: 2, Err: err}
		}
		inputs := make([]report.Input, 0, len(results))
		for _, result := range results {
			inputs = append(inputs, report.Input{Name: result.Name, Facts: result.Facts, Findings: filterFindings(result.Findings, min), Skipped: result.Skipped, Timings: result.Timings})
		}
		var b bytes.Buffer
		singleTargetReport := len(inputs) == 1 && (targetName != "" || len(results[0].Config.Targets) == 0)
		if singleTargetReport {
			if format == "json" {
				err = report.JSON(&b, Version, inputs[0].Facts, inputs[0].Findings, inputs[0].Skipped, inputs[0].Timings)
			} else {
				err = report.Text(&b, inputs[0].Facts, inputs[0].Findings, inputs[0].Skipped)
			}
		} else if format == "json" {
			err = report.JSONWorkspace(&b, Version, inputs)
		} else {
			err = report.TextWorkspace(&b, inputs)
		}
		if err != nil {
			return err
		}
		if output != "" {
			if err = os.WriteFile(output, b.Bytes(), 0o644); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
		} else {
			_, err = d.Out.Write(b.Bytes())
			if err != nil {
				return err
			}
		}
		for _, result := range results {
			for _, f := range result.Findings {
				if model.SeverityRank(f.Severity) >= model.SeverityRank(fail) {
					return &ExitError{Code: 1, Err: fmt.Errorf("findings met fail threshold %s", failOn)}
				}
			}
		}
		return nil
	}}
	cmd.Flags().StringVar(&configPath, "config", "", "explicit fleck.yaml path")
	cmd.Flags().StringVar(&targetName, "target", "", "analyze one named workspace target")
	cmd.Flags().StringSliceVar(&values, "values", nil, "Helm values overlays (replaces configured overlays)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write report to a file")
	cmd.Flags().StringVar(&rules, "rules", "", "override enabled rule IDs for selected targets")
	cmd.Flags().StringVar(&severity, "severity", "info", "minimum reported severity")
	cmd.Flags().StringVar(&failOn, "fail-on", "error", "minimum severity that returns exit status 1")
	return cmd
}

func parseSeverity(s string) (model.Severity, error) {
	switch strings.ToLower(s) {
	case "info":
		return model.SeverityInfo, nil
	case "warning":
		return model.SeverityWarning, nil
	case "error":
		return model.SeverityError, nil
	default:
		return "", fmt.Errorf("invalid severity %q (want info, warning, or error)", s)
	}
}
func filterFindings(in []model.Finding, min model.Severity) []model.Finding {
	out := make([]model.Finding, 0, len(in))
	for _, f := range in {
		if model.SeverityRank(f.Severity) >= model.SeverityRank(min) {
			out = append(out, f)
		}
	}
	return out
}
func validFormat(got string, want ...string) error {
	for _, v := range want {
		if got == v {
			return nil
		}
	}
	return fmt.Errorf("unsupported format %q (want %s)", got, strings.Join(want, " or "))
}
