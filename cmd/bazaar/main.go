package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/cli"
)

func main() {
	os.Exit(run(context.Background()))
}

func run(parent context.Context) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := cli.NewRootCmd(ctx, cli.Dependencies{})
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Error: %v\n", err); writeErr != nil {
			return cli.ExitCode(ctx, err)
		}
		return cli.ExitCode(ctx, err)
	}
	return 0
}
