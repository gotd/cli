package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
)

// envInt returns the integer value of the environment variable, or 0.
func envInt(key string) int {
	v, _ := strconv.Atoi(os.Getenv(key))
	return v
}

func newInitCmd(a *app) *cobra.Command {
	var (
		appID   int
		appHash string
		token   string
		proxy   string
		test    bool
	)

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Create the config file",
		GroupID: groupAuth,
		Long: `Create the config file at the path given by the global --config flag.

All flags are optional. Without --app-id/--app-hash, built-in Telegram Desktop
credentials are used, so you can run "tg login" right away; provide your own from
https://my.telegram.org if you prefer. A bot token is optional — most commands use
a personal user session created with "tg login". Values may also come from APP_ID,
APP_HASH and BOT_TOKEN env vars.`,
		Example: `  # Built-in credentials (then run: tg login)
  tg init

  # Your own app credentials
  tg init --app-id 10 --app-hash abcd

  # With an optional bot token
  tg init --app-id 10 --app-hash abcd --token <bot-token>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.configPath == "" {
				return errors.New("no config path provided")
			}

			cfg := Config{
				AppID:    appID,
				AppHash:  appHash,
				BotToken: token,
				Proxy:    proxy,
				Test:     test,
			}
			if err := writeConfig(a.configPath, cfg); err != nil {
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Wrote config to", a.configPath)
			return err
		},
	}

	fs := cmd.Flags()
	fs.IntVar(&appID, "app-id", envInt("APP_ID"), "telegram app ID (default: built-in)")
	fs.StringVar(&appHash, "app-hash", os.Getenv("APP_HASH"), "telegram app hash (default: built-in)")
	fs.StringVar(&token, "token", os.Getenv("BOT_TOKEN"), "optional telegram bot token")
	fs.StringVar(&proxy, "proxy", os.Getenv("TG_PROXY"), "optional proxy URL (socks5://, tg://proxy?...)")
	fs.BoolVar(&test, "test", false, "create the account against the telegram test server")

	return cmd
}
