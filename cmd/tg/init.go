package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func initFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:     "app_id",
			Required: true,
			Usage:    "Telegram app ID.",
			EnvVars:  []string{"APP_ID"},
		},
		&cli.StringFlag{
			Name:     "app_hash",
			Required: true,
			Usage:    "Telegram app hash.",
			EnvVars:  []string{"APP_HASH"},
		},
		&cli.StringFlag{
			Name:     "token",
			Required: true,
			Usage:    "Telegram bot token.",
			EnvVars:  []string{"BOT_TOKEN"},
		},
	}
}

func writeConfig(cfgPath string, cfg Config) error {
	buf := new(bytes.Buffer)
	e := yaml.NewEncoder(buf)
	e.SetIndent(2)

	if err := e.Encode(cfg); err != nil {
		return fmt.Errorf("encode: %w", err)
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

	return nil
}

func initCmd(c *cli.Context) error {
	sampleCfg := Config{
		AppID:    c.Int("app_id"),
		AppHash:  c.String("app_hash"),
		BotToken: c.String("token"),
	}

	cfgPath := c.String("config")
	if cfgPath == "" {
		return fmt.Errorf("no config path provided")
	}

	defer fmt.Println("Wrote config to", cfgPath)
	return writeConfig(cfgPath, sampleCfg)
}
