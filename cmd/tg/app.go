package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/td/telegram/dcs"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
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
	c := telegram.NewClient(p.cfg.AppID, p.cfg.AppHash, p.opts)

	return c.Run(ctx, func(ctx context.Context) error {
		s, err := c.AuthStatus(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if !s.Authorized {
			if _, err := c.AuthBot(ctx, p.cfg.BotToken); err != nil {
				return fmt.Errorf("check auth status: %w", err)
			}
		}

		return f(ctx, tg.NewClient(c))
	})
}

func (p *app) Before(c *cli.Context) error {
	// HACK for init.
	if c.Command.Name == "init" {
		return nil
	}

	cfgPath := c.String("config")
	if cfgPath == "" {
		return fmt.Errorf("no config path provided")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, &p.cfg); err != nil {
		return err
	}

	if p.cfg.BotToken == "" {
		return fmt.Errorf("no bot token provided")
	}

	// Default to same directory (near with config).
	// Probably there is better way to handle this.
	sessionName := fmt.Sprintf("gotd.session.%x.json", md5.Sum([]byte(p.cfg.BotToken)))
	p.opts.Logger = p.log.Named("tg")
	p.opts.SessionStorage = &session.FileStorage{
		Path: filepath.Join(filepath.Dir(cfgPath), sessionName),
	}
	if c.Bool("test") {
		p.opts.DCList = dcs.StagingDCs()
	}

	return nil
}
