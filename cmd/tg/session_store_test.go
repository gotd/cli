package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUseKeychain(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name string
		val  *bool
		want bool
	}{
		{"unset defaults on", nil, true},
		{"explicit on", &on, true},
		{"explicit off", &off, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &app{cfg: Config{Keychain: c.val}}
			if got := a.useKeychain(); got != c.want {
				t.Fatalf("useKeychain() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestKeychainAccount(t *testing.T) {
	acc := Account{AppID: 123}
	got := keychainAccount("work", acc, kindBot)
	want := "work." + kindBot + "." + acc.seed("work", kindBot)
	if got != want {
		t.Fatalf("keychainAccount() = %q, want %q", got, want)
	}
	// user vs bot must not collide.
	if keychainAccount("work", acc, kindUser) == got {
		t.Fatal("user and bot accounts collide")
	}
}

func TestFileSessionStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	acc := Account{AppID: 42}
	// keychain off → file backend regardless of platform.
	store := newSessionStore(dir, defaultAccount, acc, kindUser, false)

	if ok, err := store.Exists(ctx); err != nil || ok {
		t.Fatalf("Exists() = %v, %v; want false, nil", ok, err)
	}
	if err := store.StoreSession(ctx, []byte("blob")); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}
	if ok, err := store.Exists(ctx); err != nil || !ok {
		t.Fatalf("Exists() after store = %v, %v; want true, nil", ok, err)
	}
	got, err := store.LoadSession(ctx)
	if err != nil || string(got) != "blob" {
		t.Fatalf("LoadSession() = %q, %v; want \"blob\", nil", got, err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := store.Exists(ctx); ok {
		t.Fatal("Exists() after delete = true, want false")
	}
	// Delete is idempotent.
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	// File lands at the expected path.
	if _, err := os.Stat(filepath.Join(dir, "gotd.session."+defaultAccount+"."+kindUser+"."+acc.seed(defaultAccount, kindUser)+".json")); err == nil {
		t.Fatal("session file should be gone after delete")
	}
}
