package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-faster/errors"

	"github.com/gotd/cli/internal/output"
)

func main() {
	ctx, cancel := context.WithCancelCause(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		if sig, ok := <-sigs; ok {
			cancel(signalError{sig})
		}
	}()

	code := execute(ctx, os.Args[1:], os.Stdout, os.Stderr)
	signal.Stop(sigs)
	cancel(nil)
	os.Exit(code)
}

// signalError is the cancellation cause recorded when a signal stops tg.
type signalError struct{ sig os.Signal }

func (e signalError) Error() string { return "interrupted by " + e.sig.String() }

// exitCode is 128 + the signal number, the shell convention (130 for SIGINT,
// 143 for SIGTERM).
func (e signalError) exitCode() int {
	if s, ok := e.sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return exitInterrupted
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

	// A signal decides the outcome even when the command then returned nil
	// (the connection loop treats cancellation as a clean stop).
	var sig signalError
	if errors.As(context.Cause(ctx), &sig) {
		err = sig
	}
	if err == nil {
		return exitOK
	}

	info := classifyError(err)
	if errors.As(err, &sig) {
		info.Code, info.Exit = errClassInterrupted, sig.exitCode()
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
