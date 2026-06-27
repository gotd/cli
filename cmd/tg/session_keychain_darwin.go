//go:build darwin

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/go-faster/errors"

	"github.com/gotd/td/session"
)

// keychainNotFound is the exit code `security` returns when an item is absent
// (errSecItemNotFound).
const keychainNotFound = 44

// keychainSessionStore returns a Keychain-backed store. The bool is always true
// on darwin; the signature mirrors the non-darwin stub so newSessionStore can
// fall back to a file when Keychain is unavailable. legacy is the pre-Keychain
// file session path, migrated into the Keychain (and removed) on first use.
func keychainSessionStore(service, account, legacy string) (sessionStore, bool) {
	return &keychainStore{service: service, account: account, legacy: legacy}, true
}

// keychainStore stores a gotd string session as a generic password in the macOS
// login Keychain, driven through the `security` CLI (no cgo, keeps the binary
// static).
type keychainStore struct {
	service string
	account string
	legacy  string // pre-Keychain file session, migrated then deleted.
}

func keychainAddArgs(service, account, secret string) []string {
	// -U updates the item in place when it already exists.
	return []string{"add-generic-password", "-U", "-s", service, "-a", account, "-w", secret}
}

func keychainFindArgs(service, account string) []string {
	// -w prints only the password to stdout.
	return []string{"find-generic-password", "-s", service, "-a", account, "-w"}
}

func keychainDeleteArgs(service, account string) []string {
	return []string{"delete-generic-password", "-s", service, "-a", account}
}

// isNotFound reports whether err is `security` signalling a missing item.
func isNotFound(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == keychainNotFound
}

func (s *keychainStore) LoadSession(ctx context.Context) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "security", keychainFindArgs(s.service, s.account)...).Output()
	switch {
	case err == nil:
		// security appends a trailing newline to the password it prints.
		return bytes.TrimSuffix(out, []byte("\n")), nil
	case !isNotFound(err):
		return nil, errors.Wrap(err, "keychain find")
	}
	// Nothing in the Keychain: migrate a pre-Keychain file session if present.
	data, rerr := os.ReadFile(s.legacy)
	if errors.Is(rerr, os.ErrNotExist) {
		return nil, session.ErrNotFound
	}
	if rerr != nil {
		return nil, errors.Wrap(rerr, "read legacy session")
	}
	if err := s.StoreSession(ctx, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *keychainStore) StoreSession(ctx context.Context, data []byte) error {
	if err := exec.CommandContext(ctx, "security", keychainAddArgs(s.service, s.account, string(data))...).Run(); err != nil {
		return errors.Wrap(err, "keychain add")
	}
	// Any write supersedes a pre-Keychain file session; remove it best-effort.
	if err := os.Remove(s.legacy); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove legacy session")
	}
	return nil
}

func (s *keychainStore) Exists(ctx context.Context) (bool, error) {
	err := exec.CommandContext(ctx, "security", keychainFindArgs(s.service, s.account)...).Run()
	if err == nil {
		return true, nil
	}
	if !isNotFound(err) {
		return false, errors.Wrap(err, "keychain find")
	}
	// Not yet migrated: a leftover file session still counts.
	if _, serr := os.Stat(s.legacy); serr == nil {
		return true, nil
	}
	return false, nil
}

func (s *keychainStore) Delete(ctx context.Context) error {
	err := exec.CommandContext(ctx, "security", keychainDeleteArgs(s.service, s.account)...).Run()
	if err != nil && !isNotFound(err) {
		return errors.Wrap(err, "keychain delete")
	}
	// Also drop any pre-Keychain file session.
	if rerr := os.Remove(s.legacy); rerr != nil && !os.IsNotExist(rerr) {
		return errors.Wrap(rerr, "remove legacy session")
	}
	return nil
}
