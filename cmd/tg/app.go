package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/log/logzap"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
	"github.com/gotd/cli/internal/pretty"
	"github.com/gotd/cli/internal/proxy"
)

// errNotAuthorized is returned when a user-session command runs without a
// logged-in session.
var errNotAuthorized = errors.New("not authorized: run `tg login` first")

// authKind selects which session/credentials a command uses.
type authKind int

const (
	authUser authKind = iota // personal account session (default)
	authBot                  // bot token session
)

// Session kind names, used for session filenames and flags.
const (
	kindUser = "user"
	kindBot  = "bot"
)

func (k authKind) String() string {
	if k == authBot {
		return kindBot
	}
	return kindUser
}

// accountState is the resolved runtime state for the active account.
type accountState struct {
	label    string
	acc      Account
	resolver dcs.Resolver
}

// app holds shared state and the values of the global (persistent) flags.
type app struct {
	// Global flags, bound to the root command's persistent flags.
	configPath   string
	debugInvoker bool
	outputFormat string
	proxyURL     string
	accountFlag  string

	cfg     Config
	log     *zap.Logger
	waiter  *floodwait.Waiter
	printer *output.Printer

	// active is the account currently being operated on (set per run iteration).
	active *accountState

	debug bool
}

func newApp() *app {
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.Level.SetLevel(zap.WarnLevel)

	defaultLog, err := zapCfg.Build()
	if err != nil {
		panic(err)
	}

	return &app{
		waiter:  floodwait.NewWaiter(),
		log:     defaultLog,
		printer: output.New(output.Text, nil),
	}
}

// before is wired as the root command's PersistentPreRunE; it runs before every
// subcommand. It sets up the output printer and (for commands that need it)
// loads the config.
func (a *app) before(cmd *cobra.Command) error {
	format, err := output.ParseFormat(a.outputFormat)
	if err != nil {
		return err
	}
	a.printer = output.New(format, cmd.OutOrStdout())

	if skipConfig(cmd) {
		return nil
	}

	cfg, err := loadConfig(a.configPath)
	if err != nil {
		return err
	}
	a.cfg = cfg
	if a.debugInvoker {
		a.debug = true
	}
	return nil
}

// selectedLabels returns the account labels the command should run against.
func (a *app) selectedLabels() ([]string, error) {
	if a.accountFlag == "all" {
		labels := a.cfg.labels()
		if len(labels) == 0 {
			return nil, errors.New("no configured accounts")
		}
		return labels, nil
	}
	label := a.accountFlag
	if label == "" {
		// No --account / TG_ACCOUNT: use the configured default account.
		label = a.cfg.resolvedDefault()
	}
	return []string{label}, nil
}

// activate resolves and installs the active account state for label. When multi
// is set, the account label is included in output.
func (a *app) activate(label string, multi bool) error {
	st, err := a.accountState(label)
	if err != nil {
		return err
	}
	a.active = st
	if multi {
		a.printer.SetAccount(label)
	}
	return nil
}

// ensureActive activates a single selected account if none is active yet.
func (a *app) ensureActive() error {
	if a.active != nil {
		return nil
	}
	labels, err := a.selectedLabels()
	if err != nil {
		return err
	}
	if len(labels) != 1 {
		return errors.New("this command needs a single --account (not 'all')")
	}
	return a.activate(labels[0], false)
}

// skipConfig reports whether the command runs without a loaded config/session.
func skipConfig(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "init", "docs", "completion", "help",
			cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		}
	}
	return false
}

// runParams configures a client session for connect/run.
type runParams struct {
	auth    authKind
	updates bool
	// authorize performs interactive authentication when the session is not yet
	// authorized. If nil, user sessions error with errNotAuthorized and bot
	// sessions authenticate with the configured token.
	authorize func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error
}

// optionsFor builds telegram.Options for a specific account state.
func (a *app) optionsFor(st *accountState, rp runParams, d tg.UpdateDispatcher) telegram.Options {
	mw := []telegram.Middleware{a.waiter}
	if a.debug {
		mw = append(mw, pretty.Middleware())
	}

	opts := telegram.Options{
		Logger:         logzap.New(a.log.Named("tg")),
		Device:         deviceConfig(),
		Middlewares:    mw,
		SessionStorage: a.sessionStore(st.label, st.acc, rp.auth.String()),
	}
	if rp.updates {
		opts.UpdateHandler = d
	} else {
		opts.NoUpdates = true
	}
	// Test server: the global --test flag or the account's persisted setting.
	if st.acc.Test {
		opts.DCList = dcs.Test()
	}
	opts.Resolver = st.resolver
	return opts
}

// connectWith builds a client for the given account state and runs f inside the
// flood-wait + client run loop. The dispatcher is non-nil only when rp.updates
// is set.
func (a *app) connectWith(
	ctx context.Context,
	st *accountState,
	rp runParams,
	f func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error,
) error {
	var d tg.UpdateDispatcher
	if rp.updates {
		d = tg.NewUpdateDispatcher()
	}

	appID, appHash, err := effectiveCreds(st.acc)
	if err != nil {
		return err
	}
	client := telegram.NewClient(appID, appHash, a.optionsFor(st, rp, d))

	if err := a.waiter.Run(ctx, func(ctx context.Context) error {
		return client.Run(ctx, func(ctx context.Context) error {
			return f(ctx, client, d)
		})
	}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// connect builds a client for the active account and runs f. It activates a
// single selected account if none is active yet.
func (a *app) connect(
	ctx context.Context,
	rp runParams,
	f func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error,
) error {
	if err := a.ensureActive(); err != nil {
		return err
	}
	return a.connectWith(ctx, a.active, rp, f)
}

// accountState resolves an account label into runtime state (without mutating
// the shared active state), for concurrent multi-account use.
func (a *app) accountState(label string) (*accountState, error) {
	acc, err := a.cfg.account(label)
	if err != nil {
		return nil, err
	}
	proxyURL := a.proxyURL
	if proxyURL == "" {
		proxyURL = acc.Proxy
	}
	resolver, err := proxy.Resolver(proxyURL)
	if err != nil {
		return nil, err
	}
	if resolver == nil {
		// No proxy: connect like Telegram Desktop (Obfuscated2 + abridged
		// transport) instead of gotd's plain default.
		resolver = telegram.TDesktopResolver()
	}
	return &accountState{label: label, acc: acc, resolver: resolver}, nil
}

// run connects, ensures the session is authorized, and calls f with the API
// client, once per selected account. With --account all it fans out across all
// configured accounts (sequentially), labeling each result. User sessions must
// already be logged in (unless rp.authorize is set); bot sessions authenticate
// with the configured token on demand.
func (a *app) run(
	ctx context.Context,
	rp runParams,
	f func(ctx context.Context, api *tg.Client) error,
) error {
	labels, err := a.selectedLabels()
	if err != nil {
		return err
	}
	multi := len(labels) > 1

	for _, label := range labels {
		a.active = nil
		if err := a.activate(label, multi); err != nil {
			return err
		}
		if err := a.runOne(ctx, rp, f); err != nil {
			if multi {
				return errors.Wrapf(err, "account %q", label)
			}
			return err
		}
	}
	return nil
}

// runOne authorizes and runs f against the active account.
func (a *app) runOne(
	ctx context.Context,
	rp runParams,
	f func(ctx context.Context, api *tg.Client) error,
) error {
	return a.connect(ctx, rp, func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return errors.Wrap(err, "auth status")
		}
		if !status.Authorized {
			switch {
			case rp.authorize != nil:
				if err := rp.authorize(ctx, client, d); err != nil {
					return err
				}
			case rp.auth == authBot:
				if a.active.acc.BotToken == "" {
					return errors.New("no bot_token in config")
				}
				if _, err := client.Auth().Bot(ctx, a.active.acc.BotToken); err != nil {
					return errors.Wrap(err, "bot auth")
				}
			default:
				return errNotAuthorized
			}
		}
		return f(ctx, client.API())
	})
}
