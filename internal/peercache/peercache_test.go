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

func TestFindMissing(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := s.Find(context.Background(), peers.Key{Prefix: "user", ID: 1}); found {
		t.Error("expected not found")
	}
}
