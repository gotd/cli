package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/peercache"
)

// newTestManager builds a peerManager backed by a temp peer cache and the mock
// API, so id: resolution can be exercised without a network. The mock expects
// no calls; tests that need API responses build their own manager.
func newTestManager(t *testing.T) *peerManager {
	t.Helper()
	store, err := peercache.Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}
	api, _ := newTestAPI(t)
	return &peerManager{Manager: peers.Options{Storage: store}.Build(api), store: store}
}

func TestIsIDArg(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"id:42", true},
		{"  id:42  ", true},
		{"@durov", false},
		{"42", false},
		{"identity", false},
	} {
		if got := isIDArg(c.in); got != c.want {
			t.Errorf("isIDArg(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// selfAliases are the peer strings that target Saved Messages.
var selfAliases = []string{"", "me", "self", "ME"}

func TestResolvePeerSelf(t *testing.T) {
	m := newTestManager(t)
	for _, in := range selfAliases {
		p, err := resolvePeer(context.Background(), m, in)
		if err != nil {
			t.Fatalf("resolvePeer(%q): %v", in, err)
		}
		if _, ok := p.(*tg.InputPeerSelf); !ok {
			t.Errorf("resolvePeer(%q) = %T, want InputPeerSelf", in, p)
		}
	}
}

func TestResolvePeerIDNotCached(t *testing.T) {
	m := newTestManager(t)
	if _, err := resolvePeer(context.Background(), m, "id:777"); err == nil {
		t.Fatal("expected error for uncached id")
	}
}

func TestResolvePeerIDInvalid(t *testing.T) {
	m := newTestManager(t)
	if _, err := resolvePeer(context.Background(), m, "id:notanumber"); err == nil {
		t.Fatal("expected error for non-numeric id")
	}
}

func TestResolvePeerIDUser(t *testing.T) {
	ctx := context.Background()
	store, err := peercache.Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, peers.Key{Prefix: "users_", ID: 42}, peers.Value{AccessHash: 999}); err != nil {
		t.Fatal(err)
	}

	api, mock := newTestAPI(t)
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{ID: 42, AccessHash: 999, Username: "durov"},
	}})
	m := &peerManager{Manager: peers.Options{Storage: store}.Build(api), store: store}

	p, err := resolvePeer(ctx, m, "id:42")
	if err != nil {
		t.Fatal(err)
	}
	u, ok := p.(*tg.InputPeerUser)
	if !ok {
		t.Fatalf("resolvePeer = %T, want InputPeerUser", p)
	}
	if u.UserID != 42 || u.AccessHash != 999 {
		t.Errorf("input peer = %+v, want id 42 / hash 999", u)
	}
}

// TestBuilderForID guards the send path: "id:" must be resolved eagerly via the
// cache, not handed to sender.Resolve (which rejects the ":" as a bad domain).
func TestBuilderForID(t *testing.T) {
	ctx := context.Background()
	store, err := peercache.Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, peers.Key{Prefix: "users_", ID: 42}, peers.Value{AccessHash: 999}); err != nil {
		t.Fatal(err)
	}

	api, mock := newTestAPI(t)
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{ID: 42, AccessHash: 999, Username: "durov"},
	}})
	m := &peerManager{Manager: peers.Options{Storage: store}.Build(api), store: store}
	sender := message.NewSender(api).WithResolver(peerResolver{pm: m})

	if _, err := builderFor(ctx, m, sender, "id:42"); err != nil {
		t.Fatalf("builderFor(id:42): %v", err)
	}
}

func TestBuilderForSelf(t *testing.T) {
	store, err := peercache.Open(filepath.Join(t.TempDir(), "peers.json"))
	if err != nil {
		t.Fatal(err)
	}
	api, _ := newTestAPI(t)
	m := &peerManager{Manager: peers.Options{Storage: store}.Build(api), store: store}
	sender := message.NewSender(api).WithResolver(peerResolver{pm: m})
	for _, in := range selfAliases {
		if _, err := builderFor(context.Background(), m, sender, in); err != nil {
			t.Errorf("builderFor(%q): %v", in, err)
		}
	}
}
