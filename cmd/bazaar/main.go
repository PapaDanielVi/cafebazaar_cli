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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := cli.NewRootCmd(ctx, cli.Dependencies{})
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if _, writeErr := fmt.Fprintf(os.Stderr, "Error: %v\n", err); writeErr != nil {
			os.Exit(cli.ExitCode(ctx, err))
		}
		os.Exit(cli.ExitCode(ctx, err))
	}
}
