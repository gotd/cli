package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/peercache"
)

func TestResolveFilterFor(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	a := &app{
		configPath: configPath,
	}
	st := &accountState{
		label: "default",
		acc:   Account{},
	}

	// 1. Empty args
	id, err := a.resolveFilterFor(ctx, nil, st, nil)
	if err != nil {
		t.Fatalf("resolveFilterFor(nil) error: %v", err)
	}
	if id != 0 {
		t.Errorf("resolveFilterFor(nil) = %d, want 0", id)
	}

	// Pre-populate peer cache
	cachePath := st.acc.peerCachePath(filepath.Dir(configPath), st.label, authUser.String())
	store, err := peercache.Open(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, peers.Key{Prefix: "users_", ID: 777000}, peers.Value{AccessHash: 12345}); err != nil {
		t.Fatal(err)
	}

	api, mock := newTestAPI(t)

	// 2. id:777000 (cached)
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{ID: 777000, AccessHash: 12345, Username: "Telegram"},
	}})

	id, err = a.resolveFilterFor(ctx, api, st, []string{"id:777000"})
	if err != nil {
		t.Fatalf("resolveFilterFor('id:777000') error: %v", err)
	}
	if id != 777000 {
		t.Errorf("resolveFilterFor('id:777000') = %d, want 777000", id)
	}

	// 3. id:999999 (uncached)
	_, err = a.resolveFilterFor(ctx, api, st, []string{"id:999999"})
	if err == nil {
		t.Errorf("expected error for uncached id")
	}

	// 4. me / self
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{ID: 42, Self: true, Username: "myself"},
	}})

	id, err = a.resolveFilterFor(ctx, api, st, []string{peerSelfMe})
	if err != nil {
		t.Fatalf("resolveFilterFor('me') error: %v", err)
	}
	if id != 42 {
		t.Errorf("resolveFilterFor('me') = %d, want 42", id)
	}
}

func TestMessageStreamFilter(t *testing.T) {
	var captured []watchEvent
	stream := &messageStream{
		filterID: 777000,
		onEvent: func(ev watchEvent) {
			captured = append(captured, ev)
		},
	}

	// Message matching filter
	msg1 := &tg.Message{
		ID:      1,
		PeerID:  &tg.PeerUser{UserID: 777000},
		Message: "code 123456",
	}
	stream.handle(msg1, tg.Entities{})

	// Message NOT matching filter
	msg2 := &tg.Message{
		ID:      2,
		PeerID:  &tg.PeerUser{UserID: 111111},
		Message: "hello",
	}
	stream.handle(msg2, tg.Entities{})

	if len(captured) != 1 {
		t.Fatalf("captured %d events, want 1", len(captured))
	}
	if captured[0].Peer.ID != 777000 {
		t.Errorf("captured peer ID = %d, want 777000", captured[0].Peer.ID)
	}
	if captured[0].Message.Text != "code 123456" {
		t.Errorf("captured message text = %q, want 'code 123456'", captured[0].Message.Text)
	}
}
