package main

import (
	"context"
	"crypto/md5" // #nosec
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/gotd/cli/internal/pretty"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/clock"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
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

func (p *app) run(ctx context.Context, f func(ctx context.Context, api *telegram.Client) error) error {
	client := telegram.NewClient(p.cfg.AppID, p.cfg.AppHash, p.opts)

	return client.Run(ctx, func(ctx context.Context) error {
		{
			auth := client.Auth()
			s, err := auth.Status(ctx)
			if err != nil {
				return xerrors.Errorf("check auth status: %w", err)
			}
			if !s.Authorized {
				if _, err := auth.Bot(ctx, p.cfg.BotToken); err != nil {
					return xerrors.Errorf("check auth status: %w", err)
				}
			}
		}

		return f(ctx, client)
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
		p.opts.DCList = dcs.Staging()
	}
	if c.Bool("debug-invoker") {
		p.opts.Middlewares = append(p.opts.Middlewares, pretty.Middleware)
	}

	var backoffRetry telegram.MiddlewareFunc = func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			return backoff.Retry(func() error {
				if err := next.Invoke(ctx, input, output); err != nil {
					if d, ok := tgerr.AsFloodWait(err); ok {
						timer := clock.System.Timer(d + time.Second)
						defer clock.StopTimer(timer)

						select {
						case <-timer.C():
							return err
						case <-ctx.Done():
							return ctx.Err()
						}
					}

					return backoff.Permanent(err)
				}

				return nil
			}, backoff.WithContext(backoff.NewExponentialBackOff(), ctx))
		}
	}
	p.opts.Middlewares = append(p.opts.Middlewares, backoffRetry)

	return nil
}
