package peercache

import (
	"context"
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
