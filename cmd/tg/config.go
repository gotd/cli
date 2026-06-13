package main

import (
	"bytes"
	"crypto/md5" // #nosec G501 // used only to derive a stable session filename, not for security
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-faster/errors"
	"gopkg.in/yaml.v3"
)

// defaultAccount is the label of the top-level (legacy) account.
const defaultAccount = "default"

// Account holds the credentials and proxy for one Telegram account.
type Account struct {
	AppID    int    `yaml:"app_id,omitempty"`
	AppHash  string `yaml:"app_hash,omitempty"`
	BotToken string `yaml:"bot_token,omitempty"`
	// Proxy is an optional proxy URL: socks5:// or a tg://proxy?... link.
	Proxy string `yaml:"proxy,omitempty"`
	// Test connects to the Telegram test server.
	Test bool `yaml:"test,omitempty"`
}

// Config is the persisted CLI configuration.
//
// The top-level fields form the "default" account; additional named accounts
// live under `accounts`. The legacy single-account schema keeps working
// unchanged (see [[roadmap-build-decisions]]).
type Config struct {
	AppID    int    `yaml:"app_id,omitempty"`
	AppHash  string `yaml:"app_hash,omitempty"`
	BotToken string `yaml:"bot_token,omitempty"`
	Proxy    string `yaml:"proxy,omitempty"`
	Test     bool   `yaml:"test,omitempty"`

	// DefaultAccount is the account used when --account / TG_ACCOUNT is unset.
	// Empty means the top-level "default" account.
	DefaultAccount string `yaml:"default_account,omitempty"`

	// Accounts holds additional named accounts, usable via --account <label>.
	Accounts map[string]Account `yaml:"accounts,omitempty"`
}

// resolvedDefault returns the configured default account label.
func (c Config) resolvedDefault() string {
	if c.DefaultAccount != "" {
		return c.DefaultAccount
	}
	return defaultAccount
}

// defaultAcc returns the top-level account.
func (c Config) defaultAcc() Account {
	return Account{AppID: c.AppID, AppHash: c.AppHash, BotToken: c.BotToken, Proxy: c.Proxy, Test: c.Test}
}

// account returns the account config for a label ("" or "default" = top-level).
func (c Config) account(label string) (Account, error) {
	if label == "" || label == defaultAccount {
		return c.defaultAcc(), nil
	}
	a, ok := c.Accounts[label]
	if !ok {
		return Account{}, errors.Errorf("unknown account %q (see tg accounts)", label)
	}
	return a, nil
}

// labels returns all account labels, default first. The default account always
// exists (using built-in credentials when none are configured).
func (c Config) labels() []string {
	labels := []string{defaultAccount}
	keys := make([]string, 0, len(c.Accounts))
	for k := range c.Accounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return append(labels, keys...)
}

// loadConfig reads and parses the config file at path.
func loadConfig(path string) (Config, error) {
	var cfg Config
	if path == "" {
		return cfg, errors.New("no config path provided")
	}

	data, err := os.ReadFile(path) // #nosec G304 // path provided via flag
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, errors.Errorf("no config at %s; run `tg init` first", path)
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, errors.Wrap(err, "parse config")
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

// saveConfig writes cfg to path, overwriting any existing file.
func saveConfig(path string, cfg Config) error {
	buf := new(bytes.Buffer)
	e := yaml.NewEncoder(buf)
	e.SetIndent(2)
	if err := e.Encode(cfg); err != nil {
		return errors.Wrap(err, "encode")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return errors.Wrap(err, "write")
	}
	return nil
}

// sessionPath returns the session file path for an account label + auth kind.
func (a Account) sessionPath(dir, label, kind string) string {
	return filepath.Join(dir, fmt.Sprintf("gotd.session.%s.%s.%s.json", label, kind, a.seed(label, kind)))
}

// peerCachePath returns the access-hash cache path for an account label + kind.
func (a Account) peerCachePath(dir, label, kind string) string {
	return filepath.Join(dir, fmt.Sprintf("gotd.peers.%s.%s.%s.json", label, kind, a.seed(label, kind)))
}

// seed returns a stable per-account filename fragment.
func (a Account) seed(label, kind string) string {
	s := fmt.Sprintf("%s:%d:%s:%s", label, a.AppID, kind, a.BotToken)
	return fmt.Sprintf("%x", md5.Sum([]byte(s))) // #nosec G401 // filename only
}
