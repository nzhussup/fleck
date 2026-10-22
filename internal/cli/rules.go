package cli

import (
	"fmt"
	"text/tabwriter"

	detectorregistry "github.com/nzhussup/fleck/internal/detector/registry"
	"github.com/spf13/cobra"
)

func newRules(d Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "rules", Short: "Inspect analysis rules"}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List available rules", RunE: func(*cobra.Command, []string) error {
		w := tabwriter.NewWriter(d.Out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "RULE\tDESCRIPTION\tSEVERITY")
		for _, m := range detectorregistry.Metadata() {
			fmt.Fprintf(w, "%s\t%s\t%s\n", m.ID, m.Name, m.Severity)
		}
		return w.Flush()
	}})
	return root
}
