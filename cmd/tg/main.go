package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/gotd/cli/internal/output"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := execute(ctx, os.Args[1:], os.Stdout, os.Stderr)
	cancel()
	os.Exit(code)
}

// execute runs the CLI with args and returns the process exit code (see
// exitcode.go). Errors are always reported on stderr; with --output json a
// machine-readable error envelope is also written to stdout, the stream agents
// parse.
func execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root, a := newRoot()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.ExecuteContext(ctx)
	a.close()
	if err == nil {
		return exitOK
	}

	info := classifyError(err)
	if ctx.Err() != nil {
		// Stopped by a signal: whatever failed, failed because of it.
		info.Code, info.Exit = errClassInterrupted, exitInterrupted
	}
	if info.Code == errClassTimeout && a.timeout > 0 {
		info.Message = fmt.Sprintf("timed out after %s (--timeout): %s", a.timeout, info.Message)
	}

	// Concise message by default; full chain + stack with TG_DEBUG for
	// troubleshooting. All diagnostics go to stderr.
	if os.Getenv("TG_DEBUG") != "" {
		_, _ = fmt.Fprintf(stderr, "%+v\n", err)
	} else {
		_, _ = fmt.Fprintf(stderr, "tg: %s\n", info.Message)
	}
	if a.outputFormat == string(output.JSON) {
		p := a.printer
		if p.Format() != output.JSON {
			// Failed before the printer was configured (e.g. a flag error).
			p = output.New(output.JSON, stdout)
		}
		_ = p.EmitError(info)
	}
	return info.Exit
}
