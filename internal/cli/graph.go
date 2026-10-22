package cli

import (
	"fmt"
	"os"

	"github.com/nzhussup/fleck/internal/app"
	"github.com/spf13/cobra"
)

func newGraph(d Dependencies) *cobra.Command {
	var configPath, format, output string
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
		r, err := d.Service.Analyze(cmd.Context(), target, app.Options{ConfigPath: configPath, Values: overrides, SkipDetection: true})
		if err != nil {
			return &ExitError{Code: 2, Err: err}
		}
		data := []byte(r.Graph.DOT())
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
	cmd.Flags().StringSliceVar(&values, "values", nil, "Helm values overlays (replaces configured overlays)")
	cmd.Flags().StringVar(&format, "format", "dot", "graph format")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write graph to a file")
	return cmd
}
