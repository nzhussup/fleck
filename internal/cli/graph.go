package cli

import (
	"fmt"
	"os"

	"github.com/nzhussup/fleck/internal/app"
	"github.com/spf13/cobra"
)

func newGraph(d Dependencies) *cobra.Command {
	var configPath, targetName, format, output string
	var values []string
	cmd := &cobra.Command{Use: "graph [repository]", Short: "Export the configuration dependency graph", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validFormat(format, "dot"); err != nil {
			return err
		}
		target := "."
		if len(args) == 1 {
			target = args[0]
		}
		var overrides []string
		if cmd.Flags().Changed("values") {
			overrides = values
		}
		results, err := d.Service.AnalyzeAll(cmd.Context(), target, app.Options{ConfigPath: configPath, Target: targetName, Values: overrides, SkipDetection: true})
		if err != nil {
			return &ExitError{Code: 2, Err: err}
		}
		if len(results) != 1 {
			return &ExitError{Code: 2, Err: fmt.Errorf("configuration contains %d targets; select one with --target", len(results))}
		}
		data := []byte(results[0].Graph.DOT())
		if output != "" {
			if err = os.WriteFile(output, data, 0o644); err != nil {
				return fmt.Errorf("write graph: %w", err)
			}
		} else {
			_, err = d.Out.Write(data)
		}
		return err
	}}
	cmd.Flags().StringVar(&configPath, "config", "", "explicit fleck.yaml path")
	cmd.Flags().StringVar(&targetName, "target", "", "export one named workspace target")
	cmd.Flags().StringSliceVar(&values, "values", nil, "Helm values overlays (replaces configured overlays)")
	cmd.Flags().StringVar(&format, "format", "dot", "graph format")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write graph to a file")
	return cmd
}
