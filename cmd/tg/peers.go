package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/peercache"
)

// peerResolver adapts a peers.Manager (with its persistent access-hash cache) to
// the message package's peer.Resolver interface.
type peerResolver struct {
	m *peers.Manager
}

func (r peerResolver) ResolveDomain(ctx context.Context, domain string) (tg.InputPeerClass, error) {
	p, err := r.m.ResolveDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	return p.InputPeer(), nil
}

func (r peerResolver) ResolvePhone(ctx context.Context, phone string) (tg.InputPeerClass, error) {
	u, err := r.m.ResolvePhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	return u.InputPeer(), nil
}

// manager builds a peers.Manager backed by the persistent access-hash cache for
// the user account.
//
// TODO(phase7): take an account label / auth kind for multi-account.
func (a *app) manager(api *tg.Client) (*peers.Manager, error) {
	return a.managerFor(api, a.active)
}

// managerFor builds a peers.Manager for a specific account state.
func (a *app) managerFor(api *tg.Client, st *accountState) (*peers.Manager, error) {
	path := st.acc.peerCachePath(filepath.Dir(a.configPath), st.label, authUser.String())
	store, err := peercache.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "open peer cache")
	}
	return peers.Options{Storage: store}.Build(api), nil
}

// sender returns a message.Sender that resolves peers through the cached
// manager, so access-hashes persist across invocations.
func (a *app) sender(api *tg.Client) (*message.Sender, error) {
	m, err := a.manager(api)
	if err != nil {
		return nil, err
	}
	return message.NewSender(api).WithResolver(peerResolver{m: m}), nil
}

// builderFor returns a request builder targeting peer; the empty string, "me"
// and "self" target the current account's Saved Messages.
func builderFor(sender *message.Sender, peer string) *message.RequestBuilder {
	switch strings.ToLower(strings.TrimSpace(peer)) {
	case "", "me", "self":
		return sender.Self()
	default:
		return sender.Resolve(peer)
	}
}

// resolvePeer turns a peer string into an InputPeer using the cached manager.
// The empty string, "me" and "self" resolve to the current account.
func resolvePeer(ctx context.Context, m *peers.Manager, from string) (tg.InputPeerClass, error) {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "", "me", "self":
		return &tg.InputPeerSelf{}, nil
	}
	p, err := m.Resolve(ctx, from)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve peer %q", from)
	}
	return p.InputPeer(), nil
}
