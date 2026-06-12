package main

import (
	"context"
	"path/filepath"

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
// the given auth kind.
func (a *app) manager(api *tg.Client, kind authKind) (*peers.Manager, error) {
	store, err := peercache.Open(a.cfg.peerCachePath(filepath.Dir(a.configPath), kind.String()))
	if err != nil {
		return nil, errors.Wrap(err, "open peer cache")
	}
	return peers.Options{Storage: store}.Build(api), nil
}

// sender returns a message.Sender that resolves peers through the cached
// manager, so access-hashes persist across invocations.
func (a *app) sender(api *tg.Client, kind authKind) (*message.Sender, error) {
	m, err := a.manager(api, kind)
	if err != nil {
		return nil, err
	}
	return message.NewSender(api).WithResolver(peerResolver{m: m}), nil
}
