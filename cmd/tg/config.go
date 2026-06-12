package main

import (
	"bytes"
	"crypto/md5" // #nosec G501 // used only to derive a stable session filename, not for security
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
	"gopkg.in/yaml.v3"
)

// Config is the persisted CLI configuration.
//
// The schema favors a personal user account; bot_token remains optional so the
// legacy bot flows keep working (see [[roadmap-build-decisions]]).
type Config struct {
	AppID    int    `yaml:"app_id"`
	AppHash  string `yaml:"app_hash"`
	BotToken string `yaml:"bot_token,omitempty"`
	// Proxy is an optional proxy URL: socks5:// or a tg://proxy?... /
	// MTProxy link.
	Proxy string `yaml:"proxy,omitempty"`
}

// loadConfig reads and parses the config file at path.
func loadConfig(path string) (Config, error) {
	var cfg Config
	if path == "" {
		return cfg, errors.New("no config path provided")
	}

	data, err := os.ReadFile(path) // #nosec G304 // path provided via flag
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, errors.Wrap(err, "parse config")
	}
	if cfg.AppID == 0 || cfg.AppHash == "" {
		return cfg, errors.New("config is missing app_id/app_hash (run tg init)")
	}
	return cfg, nil
}

// writeConfig writes cfg to path, refusing to overwrite an existing file.
func writeConfig(path string, cfg Config) error {
	buf := new(bytes.Buffer)
	e := yaml.NewEncoder(buf)
	e.SetIndent(2)
	if err := e.Encode(cfg); err != nil {
		return errors.Wrap(err, "encode")
	}

	if _, err := os.Stat(path); err == nil {
		return errors.Errorf("file %s exists", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return errors.Wrap(err, "write")
	}
	return nil
}

// sessionPath returns the session file path for the given auth kind, derived
// from the config location and app credentials so user and bot sessions never
// collide.
func (c Config) sessionPath(dir, kind string) string {
	return filepath.Join(dir, fmt.Sprintf("gotd.session.%s.%s.json", kind, c.seed(kind)))
}

// peerCachePath returns the access-hash cache path for the given auth kind,
// kept beside the session file.
func (c Config) peerCachePath(dir, kind string) string {
	return filepath.Join(dir, fmt.Sprintf("gotd.peers.%s.%s.json", kind, c.seed(kind)))
}

// seed returns a stable per-account filename fragment.
func (c Config) seed(kind string) string {
	s := fmt.Sprintf("%d:%s:%s", c.AppID, kind, c.BotToken)
	return fmt.Sprintf("%x", md5.Sum([]byte(s))) // #nosec G401 // filename only
}
