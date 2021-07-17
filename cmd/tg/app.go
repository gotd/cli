package main

import (
	"context"
	"crypto/md5" // #nosec
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/pretty"
)

type app struct {
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

func (p *app) run(ctx context.Context, f func(ctx context.Context, api *tg.Client) error) error {
	if p.debug {
		p.opts.Middlewares = append(p.opts.Middlewares, pretty.Middleware())
	}

	client := telegram.NewClient(p.cfg.AppID, p.cfg.AppHash, p.opts)

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return p.waiter.Run(ctx)
	})
	g.Go(func() error {
		defer cancel()
		return client.Run(ctx, func(ctx context.Context) error {
			s, err := client.Auth().Status(ctx)
			if err != nil {
				return xerrors.Errorf("check auth status: %w", err)
			}
			if !s.Authorized {
				if _, err := client.Auth().Bot(ctx, p.cfg.BotToken); err != nil {
					return xerrors.Errorf("check auth status: %w", err)
				}
			}
			return f(ctx, tg.NewClient(client))
		})
	})

	if err := g.Wait(); !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (p *app) Before(c *cli.Context) error {
	// HACK for init.
	if c.Command.Name == "init" {
		return nil
	}

	cfgPath := c.String("config")
	if cfgPath == "" {
		return xerrors.Errorf("no config path provided")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, &p.cfg); err != nil {
		return err
	}

	if p.cfg.BotToken == "" {
		return xerrors.Errorf("no bot token provided")
	}

	// Default to same directory (near with config).
	// Probably there is better way to handle this.
	sessionName := fmt.Sprintf("gotd.session.%x.json", md5.Sum([]byte(p.cfg.BotToken))) // #nosec
	p.opts.Logger = p.log.Named("tg")
	p.opts.SessionStorage = &session.FileStorage{
		Path: filepath.Join(filepath.Dir(cfgPath), sessionName),
	}
	if c.Bool("test") {
		p.opts.DCList = dcs.Staging()
	}
	if c.Bool("debug-invoker") {
		p.debug = true
	}

	return nil
}
