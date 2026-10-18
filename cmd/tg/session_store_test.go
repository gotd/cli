package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/session"
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

// saveTestSession writes a gotd-format session with the given auth key id
// through store, the way the gotd client does.
func saveTestSession(t *testing.T, store session.Storage, keyID, salt int64) {
	t.Helper()
	data := &session.Data{AuthKeyID: []byte{byte(keyID)}, AuthKey: []byte{1, 2, 3}, Salt: salt, DC: 2}
	if err := (&session.Loader{Storage: store}).Save(context.Background(), data); err != nil {
		t.Fatal(err)
	}
}

func loadTestSession(t *testing.T, store session.Storage) *session.Data {
	t.Helper()
	data, err := (&session.Loader{Storage: store}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestFileSessionStoreKeepsNewerLogin(t *testing.T) {
	dir := t.TempDir()
	open := func() sessionStore { return newSessionStore(dir, "work", Account{}, kindUser, false) }

	saveTestSession(t, open(), 1, 100) // logged in with key 1
	watcher := open()                  // a long-running tg process
	loadTestSession(t, watcher)

	saveTestSession(t, open(), 2, 200) // tg login elsewhere: key 2

	// The watcher reconnects; gotd re-saves the session with its own key.
	saveTestSession(t, watcher, 1, 101)

	if got := loadTestSession(t, open()); got.AuthKeyID[0] != 2 {
		t.Errorf("auth key = %d, want the newer login (2) kept", got.AuthKeyID[0])
	}
}

func TestFileSessionStoreDoesNotResurrectLogout(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	open := func() sessionStore { return newSessionStore(dir, "work", Account{}, kindUser, false) }

	saveTestSession(t, open(), 1, 100)
	watcher := open()
	loadTestSession(t, watcher)

	if err := open().Delete(ctx); err != nil { // tg logout elsewhere
		t.Fatal(err)
	}
	saveTestSession(t, watcher, 1, 101)

	if ok, err := open().Exists(ctx); err != nil || ok {
		t.Errorf("Exists() = %v, %v; a logged-out session must stay removed", ok, err)
	}
}

func TestFileSessionStoreSameKeyLastWriterWins(t *testing.T) {
	dir := t.TempDir()
	open := func() sessionStore { return newSessionStore(dir, "work", Account{}, kindUser, false) }

	saveTestSession(t, open(), 1, 100)
	a, b := open(), open()
	loadTestSession(t, a)
	loadTestSession(t, b)

	saveTestSession(t, b, 1, 200)
	saveTestSession(t, a, 1, 300) // same key: a's newer state wins

	if got := loadTestSession(t, open()); got.Salt != 300 {
		t.Errorf("salt = %d, want 300 (last writer)", got.Salt)
	}
}

func TestFileSessionStoreOwnRekeyIsWritten(t *testing.T) {
	dir := t.TempDir()
	store := newSessionStore(dir, "work", Account{}, kindUser, false)

	saveTestSession(t, store, 1, 100) // fresh login in this process
	saveTestSession(t, store, 2, 200) // this process re-keys (e.g. tg login again)

	if got := loadTestSession(t, newSessionStore(dir, "work", Account{}, kindUser, false)); got.AuthKeyID[0] != 2 {
		t.Errorf("auth key = %d, want this process's new key (2)", got.AuthKeyID[0])
	}
}
