package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-faster/errors"

	"github.com/gotd/td/session"

	"github.com/gotd/cli/internal/atomicfile"
	"github.com/gotd/cli/internal/filelock"
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

// fileSessionStore is the cross-platform backend: the same JSON file format as
// gotd's session.FileStorage, but safe to share between tg processes, which
// agents routinely run in parallel on one account.
//
// Guarantees:
//   - Readers never see a torn file: writes replace it atomically.
//   - Writes are serialized across processes by a lock on "<file>.lock".
//   - Processes using the same auth key overwrite each other's copy (last
//     writer wins). That is harmless: the copies differ only in volatile state
//     such as the server salt, which Telegram corrects on the next request.
//   - A process never writes its auth key over a session that another process
//     re-keyed (tg login) or removed (tg logout) after this one loaded it.
//     gotd re-saves the session on every reconnect, so without this a
//     long-running `tg watch` would undo a login made elsewhere.
type fileSessionStore struct {
	path string
	mu   sync.Mutex
	// keyID is the auth key id this process uses: from the first session it
	// loaded, then from each session it stored ("" until known).
	keyID string
}

func (s *fileSessionStore) LoadSession(context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "read session")
	}
	if s.keyID == "" {
		s.keyID = sessionKeyID(data)
	}
	return data, nil
}

func (s *fileSessionStore) StoreSession(_ context.Context, data []byte) (rerr error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	unlock, err := filelock.Lock(s.path + ".lock")
	if err != nil {
		return errors.Wrap(err, "lock session")
	}
	defer func() {
		if err := unlock(); err != nil && rerr == nil {
			rerr = errors.Wrap(err, "unlock session")
		}
	}()

	newKey := sessionKeyID(data)
	if s.keyID != "" && newKey == s.keyID {
		current, err := os.ReadFile(s.path)
		if err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "read session")
		}
		if sessionKeyID(current) != s.keyID {
			// Another process logged in again or out since this one loaded
			// the session: its state is newer, keep it.
			return nil
		}
	}
	if err := atomicfile.WriteFile(s.path, data, 0o600); err != nil {
		return errors.Wrap(err, "store session")
	}
	if newKey != "" {
		s.keyID = newKey
	}
	return nil
}

// sessionKeyID extracts the auth key id from a gotd session file; "" when the
// data is empty or not in gotd's format.
func sessionKeyID(data []byte) string {
	var v struct {
		Data struct {
			AuthKeyID []byte
		}
	}
	if len(data) == 0 || json.Unmarshal(data, &v) != nil {
		return ""
	}
	return string(v.Data.AuthKeyID)
}

func (s *fileSessionStore) Exists(context.Context) (bool, error) {
	switch _, err := os.Stat(s.path); {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

func (s *fileSessionStore) Delete(context.Context) error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
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
	return &fileSessionStore{path: path}
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
