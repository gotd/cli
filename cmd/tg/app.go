package main

import (
	"context"
	"path/filepath"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/log/logzap"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
	"github.com/gotd/cli/internal/pretty"
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

// app holds shared state and the values of the global (persistent) flags.
type app struct {
	// Global flags, bound to the root command's persistent flags.
	configPath   string
	debugInvoker bool
	testServer   bool
	outputFormat string

	cfg     Config
	log     *zap.Logger
	waiter  *floodwait.Waiter
	printer *output.Printer

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

// options builds telegram.Options for the given run parameters.
func (a *app) options(rp runParams, d tg.UpdateDispatcher) telegram.Options {
	mw := []telegram.Middleware{a.waiter}
	if a.debug {
		mw = append(mw, pretty.Middleware())
	}

	opts := telegram.Options{
		Logger:      logzap.New(a.log.Named("tg")),
		Middlewares: mw,
		SessionStorage: &session.FileStorage{
			Path: a.cfg.sessionPath(filepath.Dir(a.configPath), rp.auth.String()),
		},
	}
	if rp.updates {
		opts.UpdateHandler = d
	} else {
		opts.NoUpdates = true
	}
	if a.testServer {
		opts.DCList = dcs.Test()
	}
	return opts
}

// connect builds a client per rp and runs f with the raw (possibly
// unauthorized) client inside the flood-wait + client run loop. The dispatcher
// is non-nil only when rp.updates is set.
func (a *app) connect(
	ctx context.Context,
	rp runParams,
	f func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error,
) error {
	var d tg.UpdateDispatcher
	if rp.updates {
		d = tg.NewUpdateDispatcher()
	}

	client := telegram.NewClient(a.cfg.AppID, a.cfg.AppHash, a.options(rp, d))

	if err := a.waiter.Run(ctx, func(ctx context.Context) error {
		return client.Run(ctx, func(ctx context.Context) error {
			return f(ctx, client, d)
		})
	}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// run connects, ensures the session is authorized, and calls f with the API
// client. User sessions must already be logged in (unless rp.authorize is set);
// bot sessions authenticate with the configured token on demand.
func (a *app) run(
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
				if a.cfg.BotToken == "" {
					return errors.New("no bot_token in config")
				}
				if _, err := client.Auth().Bot(ctx, a.cfg.BotToken); err != nil {
					return errors.Wrap(err, "bot auth")
				}
			default:
				return errNotAuthorized
			}
		}
		return f(ctx, client.API())
	})
}
