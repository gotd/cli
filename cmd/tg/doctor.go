package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/peercache"
)

// doctorCmdName is the doctor command, which skipConfig exempts from loading a
// config: it reports a missing or broken config itself.
const doctorCmdName = "doctor"

// doctorCheck is one diagnostic. Hint says how to fix a failed check.
type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

// doctorResult is the result of `tg doctor` for one account.
type doctorResult struct {
	Account string        `json:"account,omitempty"`
	OK      bool          `json:"ok"`
	Checks  []doctorCheck `json:"checks"`
	// exit is the code the failures map to (0 when everything passed).
	exit int
}

func (r *doctorResult) pass(name, detail string) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, OK: true, Detail: detail})
}

// fail records a failed check; exit is the code it maps to (the worst wins).
func (r *doctorResult) fail(name, detail, hint string, exit int) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, Detail: detail, Hint: hint})
	if r.exit == exitOK || exit == exitAuth {
		r.exit = exit
	}
}

// MarshalText renders one check per line.
func (r *doctorResult) MarshalText(w io.Writer) error {
	for _, c := range r.Checks {
		status := "ok  "
		if !c.OK {
			status = "FAIL"
		}
		line := fmt.Sprintf("%s %-12s %s", status, c.Name, c.Detail)
		if c.Hint != "" {
			line += " (" + c.Hint + ")"
		}
		if _, err := fmt.Fprintln(w, strings.TrimRight(line, " ")); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) newDoctorCmd() *cobra.Command {
	var offline bool

	cmd := &cobra.Command{
		Use:     doctorCmdName,
		Short:   "Diagnose the config, credentials, session and connection",
		GroupID: groupAuth,
		Long: `Check everything tg needs, in order, and say how to fix what is broken:
the config file, the selected account, app credentials, the stored session, the
peer cache, and (unless --offline) a live connection and login status.

The exit code is 0 when every check passes, 3 when the account is not logged
in, and 1 for any other failure; the result is printed either way.`,
		Example: `  tg doctor
  tg doctor -a work -o json
  tg doctor --offline`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			results := a.diagnose(cmd.Context(), offline)
			worst := exitOK
			for _, res := range results {
				if err := a.printer.Emit(res); err != nil {
					return err
				}
				if res.exit != exitOK && worst != exitAuth {
					worst = res.exit
				}
			}
			if worst != exitOK {
				return reportedError{exit: worst}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "skip the connection and login checks")
	return cmd
}

// diagnose runs the checks for every selected account.
func (a *app) diagnose(ctx context.Context, offline bool) []*doctorResult {
	cfg, err := loadConfig(a.configPath)
	if err != nil {
		res := &doctorResult{}
		res.pass("version", buildVersion().String())
		res.fail("config", err.Error(), "run `tg init`, or pass -c/TG_CONFIG", exitError)
		return []*doctorResult{res.finish()}
	}
	a.cfg = cfg

	labels, err := a.selectedLabels()
	if err != nil {
		res := &doctorResult{}
		res.fail("account", err.Error(), "", exitError)
		return []*doctorResult{res.finish()}
	}
	out := make([]*doctorResult, 0, len(labels))
	for _, label := range labels {
		out = append(out, a.diagnoseAccount(ctx, label, offline).finish())
	}
	return out
}

func (r *doctorResult) finish() *doctorResult {
	r.OK = r.exit == exitOK
	return r
}

// diagnoseAccount runs the checks for one account, stopping at the first
// failure that makes the later checks meaningless.
func (a *app) diagnoseAccount(ctx context.Context, label string, offline bool) *doctorResult {
	res := &doctorResult{Account: label}
	res.pass("version", buildVersion().String())
	res.pass("config", a.configPath)

	acc, err := a.cfg.account(label)
	if err != nil {
		res.fail("account", err.Error(), "see `tg accounts`", exitError)
		return res
	}
	res.pass("account", label)

	credsOK := a.checkCredentials(res, acc)
	sessionOK := a.checkSession(ctx, res, label, acc)
	a.checkPeerCache(res, label, acc)

	switch {
	case offline:
	case !credsOK || !sessionOK:
		res.fail("connection", "skipped", "fix the checks above first", exitError)
	default:
		a.checkConnection(ctx, res, label)
	}
	return res
}

func (a *app) checkCredentials(res *doctorResult, acc Account) bool {
	id, _, err := effectiveCreds(acc)
	switch {
	case err != nil:
		res.fail("credentials", err.Error(), "", exitError)
		return false
	case acc.AppID != 0 && acc.AppHash != "":
		res.pass("credentials", fmt.Sprintf("app %d from the config", id))
	default:
		res.pass("credentials", "built into this binary")
	}
	return true
}

func (a *app) checkSession(ctx context.Context, res *doctorResult, label string, acc Account) bool {
	store := a.sessionStore(label, acc, kindUser)
	exists, err := store.Exists(ctx)
	switch {
	case err != nil:
		res.fail("session", err.Error(), "", exitError)
		return false
	case !exists:
		res.fail("session", "no session for this account", fmt.Sprintf("log in with `tg login -a %s`", label), exitAuth)
		return false
	}
	fs, ok := store.(*fileSessionStore)
	if !ok {
		res.pass("session", "stored in the Keychain")
		return true
	}
	data, err := os.ReadFile(fs.path)
	if err != nil {
		res.fail("session", err.Error(), "", exitError)
		return false
	}
	if sessionKeyID(data) == "" {
		res.fail("session", "unreadable session file "+fs.path, fmt.Sprintf("log in again with `tg login -a %s`", label), exitAuth)
		return false
	}
	res.pass("session", "file "+fs.path)
	return true
}

func (a *app) checkPeerCache(res *doctorResult, label string, acc Account) {
	path := acc.peerCachePath(filepath.Dir(a.configPath), label, authUser.String())
	store, err := peercache.Open(path)
	if err != nil {
		res.fail("peer cache", err.Error(), "", exitError)
		return
	}
	res.pass("peer cache", fmt.Sprintf("%d cached peers", store.Len()))
}

func (a *app) checkConnection(ctx context.Context, res *doctorResult, label string) {
	st, err := a.accountState(label)
	if err != nil {
		res.fail("connection", err.Error(), "check the proxy setting", exitError)
		return
	}
	start := time.Now()
	err = a.connectWith(ctx, st, runParams{auth: authUser}, func(ctx context.Context, client *telegram.Client, _ tg.UpdateDispatcher) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return errors.Wrap(err, "auth status")
		}
		res.pass("connection", fmt.Sprintf("connected in %s", time.Since(start).Round(time.Millisecond)))
		if !status.Authorized {
			res.fail("login", "the session is not authorized", fmt.Sprintf("log in with `tg login -a %s`", label), exitAuth)
			return nil
		}
		who := fmt.Sprintf("id %d", status.User.ID)
		if status.User.Username != "" {
			who = "@" + status.User.Username + " (" + who + ")"
		}
		res.pass("login", "logged in as "+who)
		return nil
	})
	if err != nil {
		info := classifyError(err)
		res.fail("connection", info.Message, connectionHint(info), info.Exit)
	}
}

// connectionHint suggests a fix for a failed connection check.
func connectionHint(info errorInfo) string {
	switch info.Code {
	case errClassAuth:
		return "the session was revoked: log in again with `tg login`"
	case errClassTimeout:
		return "check the network or the proxy"
	case errClassFloodWait:
		return fmt.Sprintf("rate limited: retry in %ds", info.RetryAfter)
	default:
		return ""
	}
}
