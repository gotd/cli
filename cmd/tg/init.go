package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func writeConfig(cfgPath string, cfg Config) error {
	buf := new(bytes.Buffer)
	e := yaml.NewEncoder(buf)
	e.SetIndent(2)

	if err := e.Encode(cfg); err != nil {
		return errors.Wrap(err, "encode")
	}

	if _, err := os.Stat(cfgPath); err == nil {
		return errors.Errorf("file %s exist", cfgPath)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return err
	}

	if err := os.WriteFile(cfgPath, buf.Bytes(), 0o600); err != nil {
		return errors.Wrap(err, "write")
	}

	return nil
}

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
	)

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Create the config file",
		GroupID: groupAuth,
		Long: `Create the config file at the path given by the global --config flag.

Each value may instead be supplied via environment variable: APP_ID, APP_HASH,
and BOT_TOKEN.`,
		Example: `  tg init --app-id 10 --app-hash abcd --token <bot-token>`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.configPath == "" {
				return errors.New("no config path provided")
			}
			if appID == 0 || appHash == "" || token == "" {
				return errors.New("app-id, app-hash and token are required " +
					"(via flags or APP_ID/APP_HASH/BOT_TOKEN env)")
			}

			cfg := Config{
				AppID:    appID,
				AppHash:  appHash,
				BotToken: token,
			}
			if err := writeConfig(a.configPath, cfg); err != nil {
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Wrote config to", a.configPath)
			return err
		},
	}

	fs := cmd.Flags()
	fs.IntVar(&appID, "app-id", envInt("APP_ID"), "telegram app ID")
	fs.StringVar(&appHash, "app-hash", os.Getenv("APP_HASH"), "telegram app hash")
	fs.StringVar(&token, "token", os.Getenv("BOT_TOKEN"), "telegram bot token")

	return cmd
}
