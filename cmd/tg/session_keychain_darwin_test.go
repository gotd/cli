//go:build darwin

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gotd/td/session"
)

func TestKeychainArgs(t *testing.T) {
	if got := keychainAddArgs("svc", "acc", "secret"); !slices.Equal(got,
		[]string{"add-generic-password", "-U", "-s", "svc", "-a", "acc", "-w", "secret"}) {
		t.Fatalf("add args: %v", got)
	}
	if got := keychainFindArgs("svc", "acc"); !slices.Equal(got,
		[]string{"find-generic-password", "-s", "svc", "-a", "acc", "-w"}) {
		t.Fatalf("find args: %v", got)
	}
	if got := keychainDeleteArgs("svc", "acc"); !slices.Equal(got,
		[]string{"delete-generic-password", "-s", "svc", "-a", "acc"}) {
		t.Fatalf("delete args: %v", got)
	}
}

// exitErr fabricates an *exec.ExitError with the given code by actually running
// a process that exits with it (portable, no unsafe).
func exitErr(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+itoa(code)).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != code {
		t.Fatalf("could not fabricate exit %d: %v", code, err)
	}
	return err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestKeychainMigration exercises the real login Keychain (unique service,
// cleaned up). It is darwin-only and CI runs on Linux, so it never runs there.
func TestKeychainMigration(t *testing.T) {
	ctx := context.Background()
	service := "gotd.session-test-" + t.Name()
	account := "migrate"
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	store, _ := keychainSessionStore(service, account, legacy)
	t.Cleanup(func() { _ = store.Delete(ctx) })

	// A leftover file session should migrate on first load and be removed.
	want := []byte(`{"v":1,"data":"abc=="}`)
	if err := os.WriteFile(legacy, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.Exists(ctx); err != nil || !ok {
		t.Fatalf("Exists() with legacy file = %v, %v; want true, nil", ok, err)
	}
	got, err := store.LoadSession(ctx)
	if err != nil || string(got) != string(want) {
		t.Fatalf("LoadSession() = %q, %v; want %q, nil", got, err, want)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file should be removed after migration, stat err = %v", err)
	}
	// Now served from the Keychain, no file.
	got, err = store.LoadSession(ctx)
	if err != nil || string(got) != string(want) {
		t.Fatalf("LoadSession() after migration = %q, %v; want %q, nil", got, err, want)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.LoadSession(ctx); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("LoadSession() after delete = %v; want ErrNotFound", err)
	}
}

func TestIsNotFound(t *testing.T) {
	if !isNotFound(exitErr(t, keychainNotFound)) {
		t.Fatal("exit 44 should be not-found")
	}
	if isNotFound(exitErr(t, 1)) {
		t.Fatal("exit 1 should not be not-found")
	}
	if isNotFound(nil) {
		t.Fatal("nil should not be not-found")
	}
	if isNotFound(errors.New("plain")) {
		t.Fatal("non-exit error should not be not-found")
	}
}
