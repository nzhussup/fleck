package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nzhussup/fleck/internal/app"
	"github.com/spf13/cobra"
)

var Version = "0.1.0-dev"

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

type Dependencies struct {
	Service  *app.Service
	Out, Err io.Writer
}

func NewRoot(d Dependencies) *cobra.Command {
	if d.Service == nil {
		d.Service = app.New()
	}
	if d.Out == nil {
		d.Out = os.Stdout
	}
	if d.Err == nil {
		d.Err = os.Stderr
	}
	root := &cobra.Command{Use: "fleck", Short: "Catch configuration inconsistencies before deployment", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(d.Out)
	root.SetErr(d.Err)
	root.AddCommand(newCheck(d), newGraph(d), newRules(d), newVersion(d))
	return root
}
func Execute(ctx context.Context) int {
	err := NewRoot(Dependencies{}).ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		fmt.Fprintln(os.Stderr, ee.Err)
		return ee.Code
	}
	fmt.Fprintln(os.Stderr, err)
	return 2
}
