// Command tpt-mcp runs the TPT Pay Model Context Protocol server on stdio.
//
// Configure it in an MCP client with the command "tpt-mcp" and these environment variables:
//
//	TPT_API_KEY   a test secret key (sk_test_...). Needed only for create_test_payment.
//	TPT_API_BASE  API base URL. Defaults to https://api.tpt-payments.com/v1 (same as tpt).
//	TPT_DOCS      path to llms.txt. Enables the read_docs tool when set.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tpt-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base := os.Getenv("TPT_API_BASE")
	if base == "" {
		base = "https://api.tpt-payments.com/v1"
	}
	return Serve(ctx, Config{
		APIBase:  base,
		APIKey:   os.Getenv("TPT_API_KEY"),
		DocsPath: os.Getenv("TPT_DOCS"),
	}, os.Stdin, os.Stdout)
}
