package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersion(d Dependencies) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print version information", Run: func(*cobra.Command, []string) {
		fmt.Fprintf(d.Out, "fleck %s\n", Version)
	}}
}
