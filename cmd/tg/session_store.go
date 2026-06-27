package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gotd/td/session"
)

// sessionService is the Keychain service name under which sessions are stored.
// It mirrors the on-disk session filename prefix (gotd.session.*).
const sessionService = "gotd.session"

// sessionStore persists a gotd string session. It is the single seam through
// which the session is read (by the gotd client), checked for existence (by
// `accounts`), and removed (by `logout`). Backends: a plain file, or the macOS
// Keychain (default on darwin).
type sessionStore interface {
	session.Storage // LoadSession / StoreSession, consumed by telegram.Options.
	Exists(ctx context.Context) (bool, error)
	Delete(ctx context.Context) error
}

// fileSessionStore is the cross-platform backend: gotd's JSON FileStorage plus
// existence/deletion over the same path.
type fileSessionStore struct {
	*session.FileStorage
}

func (s *fileSessionStore) Exists(context.Context) (bool, error) {
	switch _, err := os.Stat(s.Path); {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

func (s *fileSessionStore) Delete(context.Context) error {
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// keychainAccount is the Keychain account name (and matches the file seed) for a
// given account label + auth kind, so user/bot and distinct app credentials
// never collide.
func keychainAccount(label string, acc Account, kind string) string {
	return label + "." + kind + "." + acc.seed(label, kind)
}

// newSessionStore selects the backend. On macOS with Keychain enabled (the
// default) it returns the Keychain backend; otherwise it falls back to a file in
// the config directory.
func newSessionStore(dir, label string, acc Account, kind string, useKeychain bool) sessionStore {
	path := acc.sessionPath(dir, label, kind)
	if useKeychain {
		// path is handed to the Keychain backend so it can migrate (and clean
		// up) a pre-Keychain file session on first use.
		if ks, ok := keychainSessionStore(sessionService, keychainAccount(label, acc, kind), path); ok {
			return ks
		}
	}
	return &fileSessionStore{FileStorage: &session.FileStorage{Path: path}}
}

// useKeychain reports whether the Keychain backend is enabled. It defaults to
// on; set `keychain: false` in the config to opt out (e.g. headless macOS).
func (a *app) useKeychain() bool {
	return a.cfg.Keychain == nil || *a.cfg.Keychain
}

// sessionStore builds the session store for an account label + auth kind.
func (a *app) sessionStore(label string, acc Account, kind string) sessionStore {
	return newSessionStore(filepath.Dir(a.configPath), label, acc, kind, a.useKeychain())
}
