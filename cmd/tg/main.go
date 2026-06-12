package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		// Concise message by default; full chain + stack with TG_DEBUG for
		// troubleshooting. All diagnostics go to stderr.
		if os.Getenv("TG_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "%+v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "tg: %v\n", err)
		}
		os.Exit(1)
	}
}
