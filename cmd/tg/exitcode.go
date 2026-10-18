package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tgerr"
)

// Process exit codes. Scripts and agents can branch on these instead of parsing
// error text. Every failure is still non-zero, so callers that only check for
// success are unaffected.
const (
	exitOK          = 0
	exitError       = 1   // unclassified failure
	exitUsage       = 2   // invalid command, flags or arguments
	exitAuth        = 3   // not logged in, or the session was revoked
	exitNotFound    = 4   // peer, message or button does not exist
	exitFloodWait   = 5   // rate limited by Telegram beyond the retry budget
	exitTimeout     = 6   // --timeout or a wait deadline elapsed
	exitRPC         = 7   // any other Telegram API error
	exitInterrupted = 130 // stopped by a signal: 128+signal, 130 for SIGINT, 143 for SIGTERM
)

// Error classes, reported as "code" in the JSON error envelope. They map 1:1 to
// the exit codes above.
const (
	errClassError       = "error"
	errClassUsage       = "usage"
	errClassAuth        = "auth"
	errClassNotFound    = "not_found"
	errClassFloodWait   = "flood_wait"
	errClassTimeout     = "timeout"
	errClassRPC         = "rpc"
	errClassInterrupted = "interrupted"
)

// errorInfo is the machine-readable description of a failed command, emitted
// as {"schema":1,"error":{...}} on stdout with --output json.
type errorInfo struct {
	Code    string `json:"code"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	// RetryAfter is the server-requested wait in seconds (flood_wait only).
	RetryAfter int `json:"retry_after,omitempty"`
	// RPC carries the Telegram error when the failure came from the API.
	RPC *rpcErrorInfo `json:"rpc,omitempty"`
}

type rpcErrorInfo struct {
	Code     int    `json:"code"`
	Type     string `json:"type"`
	Argument int    `json:"argument,omitempty"`
}

// usageError marks invalid invocations (bad flags, wrong argument count).
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

// notFoundError marks a missing peer, message or button.
type notFoundError struct{ error }

func (e notFoundError) Unwrap() error { return e.error }

// notFoundf returns an error classified as not_found.
func notFoundf(format string, args ...any) error {
	return notFoundError{errors.Errorf(format, args...)}
}

// reportedError is returned by a command that already printed its result
// (including what went wrong) and only needs a non-zero exit code; execute adds
// nothing to the output.
type reportedError struct{ exit int }

func (e reportedError) Error() string { return "exit " + strconv.Itoa(e.exit) }

// errWaitTimeout is returned when a command gave up waiting for an event.
var errWaitTimeout = errors.New("timed out waiting")

// notFoundRPCTypes are RPC error types meaning the addressed entity does not
// exist.
func notFoundRPCTypes() []string {
	return []string{
		"USERNAME_NOT_OCCUPIED", "USERNAME_INVALID", "PEER_ID_INVALID",
		"MESSAGE_ID_INVALID", "MSG_ID_INVALID", "CHANNEL_INVALID",
		"CHAT_ID_INVALID", "USER_ID_INVALID", "PHONE_NOT_OCCUPIED",
	}
}

// authRPCTypes are RPC error types meaning the session is unusable and needs
// `tg login`.
func authRPCTypes() []string {
	return []string{
		"AUTH_KEY_UNREGISTERED", "AUTH_KEY_INVALID", "AUTH_KEY_PERM_EMPTY",
		"SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED", "USER_DEACTIVATED_BAN",
	}
}

// cobraUsagePrefixes are messages of usage errors cobra raises outside the flag
// and argument validation hooks (see markUsageErrors).
func cobraUsagePrefixes() []string {
	return []string{
		"unknown command ",
		"required flag(s) ",
		"if any flags in the group ",
		"at least one of the flags in the group ",
	}
}

// classifyError maps an error to its class and exit code.
func classifyError(err error) errorInfo {
	info := errorInfo{Code: errClassError, Exit: exitError, Message: err.Error()}
	set := func(code string, exit int) errorInfo {
		info.Code, info.Exit = code, exit
		return info
	}

	if rpcErr, ok := tgerr.As(err); ok {
		info.RPC = &rpcErrorInfo{Code: rpcErr.Code, Type: rpcErr.Type, Argument: rpcErr.Argument}
	}

	var (
		usage    usageError
		notFound notFoundError
	)
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, errWaitTimeout):
		return set(errClassTimeout, exitTimeout)
	case errors.Is(err, context.Canceled):
		return set(errClassInterrupted, exitInterrupted)
	case errors.As(err, &usage) || hasAnyPrefix(err.Error(), cobraUsagePrefixes()):
		return set(errClassUsage, exitUsage)
	case errors.Is(err, errNotAuthorized) || tgerr.Is(err, authRPCTypes()...) || tgerr.IsCode(err, 401):
		return set(errClassAuth, exitAuth)
	case errors.As(err, &notFound) || tgerr.Is(err, notFoundRPCTypes()...):
		return set(errClassNotFound, exitNotFound)
	}
	if d, ok := tgerr.AsFloodWait(err); ok {
		info.RetryAfter = int(d.Seconds())
		return set(errClassFloodWait, exitFloodWait)
	}
	if rpcErr, ok := tgerr.AsType(err, "SLOWMODE_WAIT"); ok {
		info.RetryAfter = rpcErr.Argument
		return set(errClassFloodWait, exitFloodWait)
	}
	if info.RPC != nil {
		return set(errClassRPC, exitRPC)
	}
	return info
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// markUsageErrors classifies flag-parsing and argument-validation failures of
// cmd and all its subcommands as usage errors.
func markUsageErrors(cmd *cobra.Command) {
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})
	var mark func(c *cobra.Command)
	mark = func(c *cobra.Command) {
		if args := c.Args; args != nil {
			c.Args = func(c *cobra.Command, a []string) error {
				if err := args(c, a); err != nil {
					return usageError{err}
				}
				return nil
			}
		}
		for _, sub := range c.Commands() {
			mark(sub)
		}
	}
	mark(cmd)
}

// parseMessageID parses a <message-id> argument.
func parseMessageID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, usageError{errors.Errorf("message-id must be a positive integer, got %q", s)}
	}
	return id, nil
}
