package peercache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/telegram/peers"
)

func TestStorageRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peers.json")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	key := peers.Key{Prefix: "user", ID: 42}
	if err := s.Save(ctx, key, peers.Value{AccessHash: 999}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePhone(ctx, "+100", key); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveContactsHash(ctx, 7); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk to confirm persistence.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	v, found, err := s2.Find(ctx, key)
	if err != nil || !found || v.AccessHash != 999 {
		t.Fatalf("Find = %+v, %v, %v", v, found, err)
	}

	gotKey, gotVal, found, err := s2.FindPhone(ctx, "+100")
	if err != nil || !found {
		t.Fatalf("FindPhone found=%v err=%v", found, err)
	}
	if gotKey != key || gotVal.AccessHash != 999 {
		t.Errorf("FindPhone = %+v / %+v, want %+v / hash 999", gotKey, gotVal, key)
	}

	hash, err := s2.GetContactsHash(ctx)
	if err != nil || hash != 7 {
		t.Errorf("GetContactsHash = %d, %v", hash, err)
	}
}

func TestKind(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Saved under the prefixes gotd's peers.Manager uses.
	save := func(prefix string, id, hash int64) {
		if err := s.Save(ctx, peers.Key{Prefix: prefix, ID: id}, peers.Value{AccessHash: hash}); err != nil {
			t.Fatal(err)
		}
	}
	save("users_", 42, 111)
	save("channel_", 7, 222)
	save("chats_", 9, 0) // basic groups have no access hash

	cases := []struct {
		id   int64
		want string
	}{
		{42, KindUser},
		{7, KindChannel},
		{9, KindChat},
	}
	for _, c := range cases {
		got, ok := s.Kind(c.id)
		if !ok || got != c.want {
			t.Errorf("Kind(%d) = %q, %v; want %q", c.id, got, ok, c.want)
		}
	}
	if got, ok := s.Kind(99999); ok {
		t.Errorf("Kind(99999) = %q, %v; want not found", got, ok)
	}
}

func TestFindMissing(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := s.Find(context.Background(), peers.Key{Prefix: "user", ID: 1}); found {
		t.Error("expected not found")
	}
}

func TestConcurrentWritersMerge(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peers.json")

	// Two processes open the same (empty) cache, then each saves a different peer.
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Save(ctx, peers.Key{Prefix: "users_", ID: 1}, peers.Value{AccessHash: 11}); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(ctx, peers.Key{Prefix: "users_", ID: 2}, peers.Value{AccessHash: 22}); err != nil {
		t.Fatal(err)
	}

	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int64{1: 11, 2: 22} {
		v, found, err := c.Find(ctx, peers.Key{Prefix: "users_", ID: id})
		if err != nil || !found || v.AccessHash != want {
			t.Errorf("peer %d: Find = %+v, %v, %v; want hash %d", id, v, found, err, want)
		}
	}
}

func TestCorruptCacheIsRebuilt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peers.json")
	// A torn write from an older version: truncated JSON.
	if err := os.WriteFile(path, []byte(`{"peers":{"users_:1":`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open should tolerate a corrupt cache: %v", err)
	}
	key := peers.Key{Prefix: "users_", ID: 5}
	if err := s.Save(ctx, key, peers.Value{AccessHash: 55}); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, found, _ := s2.Find(ctx, key); !found || v.AccessHash != 55 {
		t.Errorf("Find after rebuild = %+v, %v", v, found)
	}
}
