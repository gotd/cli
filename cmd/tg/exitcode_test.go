package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/tgerr"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		code       string
		exit       int
		retryAfter int
		rpcType    string
	}{
		{"generic", errors.New("boom"), errClassError, exitError, 0, ""},
		{"usage", usageError{errors.New("bad flag")}, errClassUsage, exitUsage, 0, ""},
		{"cobra unknown command", errors.New(`unknown command "x" for "tg"`), errClassUsage, exitUsage, 0, ""},
		{"not authorized", errors.Wrap(errNotAuthorized, "run"), errClassAuth, exitAuth, 0, ""},
		{"session revoked", errors.Wrap(tgerr.New(401, "SESSION_REVOKED"), "rpc"), errClassAuth, exitAuth, 0, "SESSION_REVOKED"},
		{"not found sentinel", notFoundf("message #%d not found", 7), errClassNotFound, exitNotFound, 0, ""},
		{"username not occupied", tgerr.New(400, "USERNAME_NOT_OCCUPIED"), errClassNotFound, exitNotFound, 0, "USERNAME_NOT_OCCUPIED"},
		{"flood wait", errors.Wrap(tgerr.New(420, "FLOOD_WAIT_42"), "send"), errClassFloodWait, exitFloodWait, 42, "FLOOD_WAIT"},
		{"slow mode", tgerr.New(420, "SLOWMODE_WAIT_9"), errClassFloodWait, exitFloodWait, 9, "SLOWMODE_WAIT"},
		{"other rpc", tgerr.New(400, "MESSAGE_TOO_LONG"), errClassRPC, exitRPC, 0, "MESSAGE_TOO_LONG"},
		{"deadline", errors.Wrap(context.DeadlineExceeded, "rpc"), errClassTimeout, exitTimeout, 0, ""},
		{"wait timeout", errors.Wrap(errWaitTimeout, "no message"), errClassTimeout, exitTimeout, 0, ""},
		{"canceled", context.Canceled, errClassInterrupted, exitInterrupted, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyError(c.err)
			if got.Code != c.code || got.Exit != c.exit || got.RetryAfter != c.retryAfter {
				t.Errorf("classifyError() = %+v; want code=%s exit=%d retry_after=%d", got, c.code, c.exit, c.retryAfter)
			}
			if c.rpcType == "" && got.RPC != nil {
				t.Errorf("unexpected rpc info %+v", got.RPC)
			}
			if c.rpcType != "" && (got.RPC == nil || got.RPC.Type != c.rpcType) {
				t.Errorf("rpc = %+v, want type %s", got.RPC, c.rpcType)
			}
			if got.Message != c.err.Error() {
				t.Errorf("message = %q, want %q", got.Message, c.err.Error())
			}
		})
	}
}

// runCLI runs the CLI in-process with an empty (but present) config and no
// sessions, so commands get as far as needing a login.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gotd.cli.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return runCLIConfig(t, path, args...)
}

// runCLIConfig runs the CLI in-process with the given config path.
func runCLIConfig(t *testing.T, configPath string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	args = append([]string{"--config", configPath}, args...)
	code = execute(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestExecuteUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown command": {"no-such-command"},
		"unknown flag":    {"history", "--no-such-flag", "me"},
		"missing args":    {"history"},
		"bad message id":  {"context", "me", "abc"},
		"no selector":     {"click", "@bot"},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, stderr := runCLI(t, args...)
			if code != exitUsage {
				t.Errorf("exit = %d, want %d (stderr: %s)", code, exitUsage, stderr)
			}
			if !strings.HasPrefix(stderr, "tg: ") {
				t.Errorf("stderr = %q, want a tg: prefixed message", stderr)
			}
		})
	}
}

func TestExecuteJSONErrorEnvelope(t *testing.T) {
	code, stdout, _ := runCLI(t, "-o", "json", "history")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	var env struct {
		Schema int        `json:"schema"`
		Error  *errorInfo `json:"error"`
		Data   any        `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout)
	}
	if env.Schema != 1 || env.Error == nil || env.Error.Code != errClassUsage || env.Error.Exit != exitUsage || env.Data != nil {
		t.Errorf("unexpected envelope: %s", stdout)
	}
}

func TestExecuteTextModeKeepsStdoutClean(t *testing.T) {
	code, stdout, _ := runCLI(t, "history")
	if code != exitUsage || stdout != "" {
		t.Errorf("exit = %d, stdout = %q; want usage exit and empty stdout", code, stdout)
	}
}

func TestExecuteMissingConfig(t *testing.T) {
	// A command that needs a config fails cleanly (generic error) without one.
	code, _, stderr := runCLIConfig(t, filepath.Join(t.TempDir(), "missing.yaml"), "whoami")
	if code != exitError || !strings.Contains(stderr, "tg init") {
		t.Errorf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestExecuteVersion(t *testing.T) {
	code, stdout, stderr := runCLI(t, "version", "-o", "json")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	var env struct {
		Data versionInfo `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || env.Data.Version == "" {
		t.Fatalf("bad version output %q: %v", stdout, err)
	}

	code, stdout, _ = runCLI(t, "--version")
	if code != exitOK || !strings.HasPrefix(stdout, "tg ") {
		t.Errorf("--version: exit = %d, stdout = %q", code, stdout)
	}
}

func TestWaitTimeoutFlagShadowsGlobal(t *testing.T) {
	// `wait` keeps its own --timeout; the global one must not break parsing.
	code, _, stderr := runCLI(t, "wait", "--timeout", "1s", "--help")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	code, _, stderr = runCLI(t, "--timeout", "1s", "history", "--help")
	if code != exitOK {
		t.Fatalf("global --timeout: exit = %d, stderr = %s", code, stderr)
	}
}

func TestParseMessageID(t *testing.T) {
	for in, want := range map[string]int{"1": 1, "4242": 4242} {
		if got, err := parseMessageID(in); err != nil || got != want {
			t.Errorf("parseMessageID(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0", "-3", "1.5", "abc", " 7"} {
		_, err := parseMessageID(in)
		var u usageError
		if !errors.As(err, &u) {
			t.Errorf("parseMessageID(%q) err = %v, want usage error", in, err)
		}
	}
}

func TestConfigPathFromEnv(t *testing.T) {
	t.Setenv("TG_CONFIG", "/custom/tg.yaml")
	if got := defaultConfigPath(); got != "/custom/tg.yaml" {
		t.Errorf("defaultConfigPath() = %q, want $TG_CONFIG", got)
	}
}

func TestExecuteInterrupted(t *testing.T) {
	// Regression: a command that returns nil after cancellation (the
	// connection loop swallows context.Canceled) exited 0 on Ctrl-C.
	for sig, want := range map[syscall.Signal]int{syscall.SIGINT: 130, syscall.SIGTERM: 143} {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(signalError{sig})
		var out, errOut bytes.Buffer
		code := execute(ctx, []string{"-o", "json", "version"}, &out, &errOut)
		if code != want {
			t.Errorf("%v: exit = %d, want %d", sig, code, want)
		}
		if !strings.Contains(errOut.String(), "interrupted") || !strings.Contains(out.String(), `"code": "interrupted"`) {
			t.Errorf("%v: stderr = %q, stdout = %q", sig, errOut.String(), out.String())
		}
	}
}
