package main

import (
	"context"
	"crypto/md5" // #nosec
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/log/logzap"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/pretty"
)

// Config is the persisted CLI configuration.
type Config struct {
	AppID    int    `yaml:"app_id"`
	AppHash  string `yaml:"app_hash"`
	BotToken string `yaml:"bot_token"`
}

// app holds shared state and the values of the global (persistent) flags.
type app struct {
	// Global flags, bound to the root command's persistent flags.
	configPath   string
	debugInvoker bool
	testServer   bool

	cfg    Config
	log    *zap.Logger
	waiter *floodwait.Waiter

	opts  telegram.Options
	debug bool
}

func newApp() *app {
	// TODO(ernado): We need to log somewhere until configured?
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.Level.SetLevel(zap.WarnLevel)

	defaultLog, err := zapCfg.Build()
	if err != nil {
		panic(err)
	}

	waiter := floodwait.NewWaiter()

	return &app{
		waiter: waiter,
		log:    defaultLog,
		opts: telegram.Options{
			Middlewares: []telegram.Middleware{
				waiter,
			},
			NoUpdates: true,
		},
	}
}

func (a *app) run(ctx context.Context, f func(ctx context.Context, api *tg.Client) error) error {
	if a.debug {
		a.opts.Middlewares = append(a.opts.Middlewares, pretty.Middleware())
	}

	client := telegram.NewClient(a.cfg.AppID, a.cfg.AppHash, a.opts)

	if err := a.waiter.Run(ctx, func(ctx context.Context) error {
		return client.Run(ctx, func(ctx context.Context) error {
			s, err := client.Auth().Status(ctx)
			if err != nil {
				return errors.Wrap(err, "check auth status")
			}
			if !s.Authorized {
				if _, err := client.Auth().Bot(ctx, a.cfg.BotToken); err != nil {
					return errors.Wrap(err, "check auth status")
				}
			}
			return f(ctx, tg.NewClient(client))
		})
	}); !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// skipConfigCommands are commands that must run without a loaded config/session.
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

// before loads config and prepares client options. It is wired as the root
// command's PersistentPreRunE, so it runs before every subcommand.
func (a *app) before(cmd *cobra.Command) error {
	if skipConfig(cmd) {
		return nil
	}

	if a.configPath == "" {
		return errors.New("no config path provided")
	}

	data, err := os.ReadFile(a.configPath) // #nosec G304 // path provided via flag
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, &a.cfg); err != nil {
		return err
	}

	if a.cfg.BotToken == "" {
		return errors.New("no bot token provided")
	}

	// Default to same directory (near with config).
	// Probably there is better way to handle this.
	sessionName := fmt.Sprintf("gotd.session.%x.json", md5.Sum([]byte(a.cfg.BotToken))) // #nosec
	a.opts.Logger = logzap.New(a.log.Named("tg"))
	a.opts.SessionStorage = &session.FileStorage{
		Path: filepath.Join(filepath.Dir(a.configPath), sessionName),
	}
	if a.testServer {
		a.opts.DCList = dcs.Test()
	}
	if a.debugInvoker {
		a.debug = true
	}

	return nil
}
