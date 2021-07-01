package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/urfave/cli/v2"
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
	p := newApp()
	app := &cli.App{
		Name:  "tg",
		Usage: "Telegram CLI",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   defaultConfigPath(),
				Usage:   "config to use",
			},
			&cli.BoolFlag{
				Name:  "debug-invoker",
				Usage: "use pretty-printing debug invoker",
			},
			&cli.BoolFlag{
				Name:    "test",
				Aliases: []string{"staging"},
				Usage:   "connect to telegram test server",
			},
		},

		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Creates config file",
				Description: `Command init creates config file at the given path.
Example:
	tg init --app-id 10 --app-hash abcd --token token
`,
				Flags:  initFlags(),
				Action: initCmd,
			},
			{
				Name:      "send",
				Usage:     "Sends message to peer",
				ArgsUsage: "[message]",
				Flags:     p.sendFlags(),
				Action:    p.sendCmd,
			},
			{
				Name:      "upload",
				Aliases:   []string{"up"},
				Usage:     "Uploads file to peer",
				ArgsUsage: "[path]",
				Flags:     p.uploadFlags(),
				Action:    p.uploadCmd,
			},
			{
				Name:  "sticker",
				Usage: "Manage sticker sets",
				Subcommands: []*cli.Command{
					{
						Name:      "add",
						Usage:     "Uploads and adds sticker to sticker set",
						ArgsUsage: "[path]",
						Flags:     p.stickerAddFlags(),
						Action:    p.stickerAddCmd,
					},
					{
						Name:      "create",
						Usage:     "Creates new sticker set",
						ArgsUsage: "[shortname]",
						Flags:     p.stickerCreateFlags(),
						Action:    p.stickerCreateCmd,
					},
				},
			},
		},
	}
	for _, cmd := range app.Commands {
		cmd.Before = p.Before
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}
