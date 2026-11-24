package cli

import (
	"context"
	"runtime"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func newVersionCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	return &cobra.Command{
		Use:   "version",
		Short: "Print bazaar CLI version and build metadata",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			info := map[string]string{
				"version":    Version,
				"commit":     Commit,
				"build_date": BuildDate,
				"go_version": runtime.Version(),
				"os_arch":    runtime.GOOS + "/" + runtime.GOARCH,
			}
			if printed, err := rt.Printer.PrintStructured(c.Context(), info); printed || err != nil {
				return err
			}
			return rt.Printer.PrintCard(c.Context(), "bazaar CLI", []output.KeyValue{
				{Key: labelVersion, Value: Version},
				{Key: "Commit", Value: Commit},
				{Key: "Built", Value: BuildDate},
				{Key: "Runtime", Value: runtime.Version() + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"},
			})
		},
	}
}
