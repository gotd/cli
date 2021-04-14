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

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/invokers"
	"github.com/gotd/td/tg"
)

type app struct {
	cfg  Config
	log  *zap.Logger
	opts telegram.Options
}

func newApp() *app {
	// TODO(ernado): We need to log somewhere until configured?
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.Level.SetLevel(zap.WarnLevel)

	defaultLog, err := zapCfg.Build()
	if err != nil {
		panic(err)
	}

	return &app{
		log: defaultLog,
		opts: telegram.Options{
			NoUpdates: true,
		},
	}
}

func (p *app) run(ctx context.Context, f func(ctx context.Context, api *tg.Client) error) error {
	client := telegram.NewClient(p.cfg.AppID, p.cfg.AppHash, p.opts)

	return client.Run(ctx, func(ctx context.Context) error {
		s, err := client.AuthStatus(ctx)
		if err != nil {
			return xerrors.Errorf("check auth status: %w", err)
		}
		if !s.Authorized {
			if _, err := client.AuthBot(ctx, p.cfg.BotToken); err != nil {
				return xerrors.Errorf("check auth status: %w", err)
			}
		}

		invoker := invokers.NewWaiter(client)
		raw := tg.NewClient(invoker)

		ctx, cancel := context.WithCancel(ctx)
		g, ctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			return invoker.Run(ctx)
		})
		g.Go(func() error {
			defer cancel()
			return f(ctx, raw)
		})

		if err := g.Wait(); !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})
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
		p.opts.DCList = dcs.StagingDCs()
	}

	return nil
}
