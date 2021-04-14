package main

import (
	"context"
	stdlog "log"
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
				Usage:   "Config to use",
			},
		},

		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "init config file",
				Description: `Command init creates config file at the given path.
Example:
	tg init --app_id 10 --app_hash abcd --token token
`,
				Flags:  initFlags(),
				Action: initCmd,
			},
			{
				Name:      "send",
				Usage:     "Send message to peer",
				ArgsUsage: "[message]",
				Flags:     p.sendFlags(),
				Action:    p.sendCmd,
			},
			{
				Name:      "upload",
				Aliases:   []string{"up"},
				Usage:     "upload file to peer",
				ArgsUsage: "[path]",
				Flags:     p.uploadFlags(),
				Action:    p.uploadCmd,
			},
		},
	}
	for _, cmd := range app.Commands {
		cmd.Before = p.Before
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := app.RunContext(ctx, os.Args); err != nil {
		stdlog.Fatalf("Run: %+v", err)
	}
}
