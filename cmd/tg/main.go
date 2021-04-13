package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

type Config struct {
	AppID    int    `yaml:"app_id"`
	AppHash  string `yaml:"app_hash"`
	BotToken string `yaml:"bot_token"`
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "gotd", "gotd.cli.yaml")
}

func main() {
	var (
		cfg Config
		log *zap.Logger

		opt = telegram.Options{
			NoUpdates: true,
		}
	)

	{
		// We need to log somewhere until configured?
		zapCfg := zap.NewDevelopmentConfig()
		zapCfg.Level.SetLevel(zap.WarnLevel)

		defaultLog, err := zapCfg.Build()
		if err != nil {
			panic(err)
		}
		log = defaultLog
	}

	run := func(ctx context.Context, f func(ctx context.Context, api *tg.Client) error) error {
		c := telegram.NewClient(cfg.AppID, cfg.AppHash, opt)

		return c.Run(ctx, func(ctx context.Context) error {
			s, err := c.AuthStatus(ctx)
			if err != nil {
				return err
			}
			if !s.Authorized {
				if _, err := c.AuthBot(ctx, cfg.BotToken); err != nil {
					return err
				}
			}

			return f(ctx, tg.NewClient(c))
		})
	}

	app := &cli.App{
		Name:  "tg",
		Usage: "Telegram CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   defaultConfigPath(),
				Usage:   "Config to use",
			},
		},

		Before: func(c *cli.Context) error {
			// HACK for init.
			if len(os.Args) >= 2 && os.Args[1] == "init" {
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

			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return err
			}

			if cfg.BotToken == "" {
				return fmt.Errorf("no bot token provided")
			}

			// Default to same directory (near with config).
			// Probably there is better way to handle this.
			sessionName := fmt.Sprintf("gotd.session.%x.json", md5.Sum([]byte(cfg.BotToken)))
			opt.Logger = log.Named("tg")
			opt.SessionStorage = &session.FileStorage{
				Path: filepath.Join(filepath.Dir(cfgPath), sessionName),
			}

			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "init config file",
				Action: func(c *cli.Context) error {
					buf := new(bytes.Buffer)
					e := yaml.NewEncoder(buf)
					e.SetIndent(2)

					sampleCfg := Config{
						AppID:    telegram.TestAppID,
						AppHash:  telegram.TestAppHash,
						BotToken: "123456:10",
					}
					if err := e.Encode(sampleCfg); err != nil {
						return err
					}

					cfgPath := c.String("config")
					if cfgPath == "" {
						return fmt.Errorf("no config path provided")
					}

					if _, err := os.Stat(cfgPath); err == nil {
						return fmt.Errorf("file %s exist", cfgPath)
					}

					if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
						return err
					}

					if err := os.WriteFile(cfgPath, buf.Bytes(), 0600); err != nil {
						return fmt.Errorf("write: %w", err)
					}

					fmt.Println("Wrote sample config to", cfgPath)

					return nil
				},
			},
			{
				Name:      "upload",
				Aliases:   []string{"up"},
				Usage:     "upload file to peer",
				ArgsUsage: "[path]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "peer",
						Aliases: []string{"p", "target"},
						Usage:   "Peer to write (e.g. channel name or username)",
					},
				},
				Action: func(c *cli.Context) error {
					return run(c.Context, func(ctx context.Context, api *tg.Client) error {
						name := c.Args().First()
						if name == "" {
							return errors.New("no file name provided")
						}
						f, err := os.Open(c.Args().First())
						if err != nil {
							return err
						}
						defer func() {
							_ = f.Close()
						}()

						var target tg.InputPeerClass = &tg.InputPeerSelf{}
						if targetDomain := c.String("peer"); targetDomain != "" {
							r := peer.DefaultResolver(api)
							resolved, err := r.ResolveDomain(ctx, targetDomain)
							if err != nil {
								return fmt.Errorf("failed to resolve %s: %w", targetDomain, err)
							}
							target = resolved
							fmt.Println("Uploading", f.Name(), "to", targetDomain)
						} else {
							fmt.Println("Saving", f.Name(), "to favorites")
						}

						s, err := f.Stat()
						if err != nil {
							return err
						}

						p := progressbar.DefaultBytes(s.Size(), "upload")

						u := uploader.NewUploader(api)

						upload := uploader.NewUpload(filepath.Base(f.Name()), io.TeeReader(f, p), s.Size())

						fileInput, err := u.Upload(ctx, upload)
						if err != nil {
							return err
						}

						if _, err := message.NewSender(api).
							To(target).
							File(ctx, fileInput, styling.Plain(f.Name())); err != nil {
							return err
						}

						return nil
					})
				},
			},
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		stdlog.Fatalf("Run: %+v", err)
	}
}
