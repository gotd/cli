package main

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/peercache"
)

// peerIDPrefix marks a peer argument as a raw numeric id, e.g. "id:2201861038".
// It lets you address a peer that has no username or phone (as shown in the
// "id" field of `tg chats list --output json`), provided the peer has been
// cached (its access hash is stored) by a prior command like `tg chats list`.
const peerIDPrefix = "id:"

// peerManager bundles a peers.Manager with the cache store it is built on, so
// peer resolution can consult the cache for "id:" lookups. It embeds the
// manager, so all of its methods remain available directly.
type peerManager struct {
	*peers.Manager
	store *peercache.Storage
}

// resolveID resolves an "id:<n>" argument to a cached peer. The kind
// (user/chat/channel) comes from the peer cache; an unseen id is an error that
// points the user at `tg chats list`.
func (m *peerManager) resolveID(ctx context.Context, arg string) (peers.Peer, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(arg, peerIDPrefix))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid peer id %q", raw)
	}
	kind, ok := m.store.Kind(id)
	if !ok {
		return nil, errors.Errorf("peer id %d not in cache; run `tg chats list` (or `tg contacts list`) first so its access hash is stored", id)
	}
	switch kind {
	case peercache.KindUser:
		return m.ResolveUserID(ctx, id)
	case peercache.KindChannel:
		return m.ResolveChannelID(ctx, id)
	case peercache.KindChat:
		return m.ResolveChatID(ctx, id)
	default:
		return nil, errors.Errorf("unknown peer kind %q for id %d", kind, id)
	}
}

// isIDArg reports whether arg uses the "id:" prefix.
func isIDArg(arg string) bool {
	return strings.HasPrefix(strings.TrimSpace(arg), peerIDPrefix)
}

// isSelf reports whether a peer string targets the current account's Saved
// Messages: the empty string, "me" or "self" (case-insensitive).
func isSelf(peer string) bool {
	switch strings.ToLower(strings.TrimSpace(peer)) {
	case "", "me", "self":
		return true
	default:
		return false
	}
}

// peerResolver adapts a peerManager (with its persistent access-hash cache) to
// the message package's peer.Resolver interface, adding "id:" support.
type peerResolver struct {
	pm *peerManager
}

func (r peerResolver) ResolveDomain(ctx context.Context, domain string) (tg.InputPeerClass, error) {
	if isIDArg(domain) {
		p, err := r.pm.resolveID(ctx, domain)
		if err != nil {
			return nil, err
		}
		return p.InputPeer(), nil
	}
	p, err := r.pm.ResolveDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	return p.InputPeer(), nil
}

func (r peerResolver) ResolvePhone(ctx context.Context, phone string) (tg.InputPeerClass, error) {
	u, err := r.pm.ResolvePhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	return u.InputPeer(), nil
}

// manager builds a peerManager backed by the persistent access-hash cache for
// the user account.
//
// TODO(phase7): take an account label / auth kind for multi-account.
func (a *app) manager(api *tg.Client) (*peerManager, error) {
	return a.managerFor(api, a.active)
}

// managerFor builds a peerManager for a specific account state.
func (a *app) managerFor(api *tg.Client, st *accountState) (*peerManager, error) {
	path := st.acc.peerCachePath(filepath.Dir(a.configPath), st.label, authUser.String())
	store, err := peercache.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "open peer cache")
	}
	return &peerManager{
		Manager: peers.Options{Storage: store}.Build(api),
		store:   store,
	}, nil
}

// sender returns a message.Sender that resolves peers through the cached
// manager, so access-hashes persist across invocations.
func (a *app) sender(api *tg.Client) (*message.Sender, error) {
	m, err := a.manager(api)
	if err != nil {
		return nil, err
	}
	return message.NewSender(api).WithResolver(peerResolver{pm: m}), nil
}

// builderFor returns a request builder targeting peer; the empty string, "me"
// and "self" target the current account's Saved Messages.
func builderFor(sender *message.Sender, peer string) *message.RequestBuilder {
	if isSelf(peer) {
		return sender.Self()
	}
	return sender.Resolve(peer)
}

// resolvePeer turns a peer string into an InputPeer using the cached manager.
// The empty string, "me" and "self" resolve to the current account; an "id:<n>"
// argument resolves a cached peer by numeric id.
func resolvePeer(ctx context.Context, m *peerManager, from string) (tg.InputPeerClass, error) {
	if isSelf(from) {
		return &tg.InputPeerSelf{}, nil
	}
	if isIDArg(from) {
		p, err := m.resolveID(ctx, from)
		if err != nil {
			return nil, err
		}
		return p.InputPeer(), nil
	}
	p, err := m.Resolve(ctx, from)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve peer %q", from)
	}
	return p.InputPeer(), nil
}
