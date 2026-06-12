package main

import (
	"strconv"
	"strings"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
)

// Peer type names.
const (
	peerUser    = "user"
	peerChat    = "chat"
	peerChannel = "channel"
	peerUnknown = "unknown"
)

// peerRef is a stable, compact description of a peer.
type peerRef struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
}

// label renders a peer reference for text output, e.g. "@durov" or "Some Group".
func (p peerRef) label() string {
	switch {
	case p.Username != "":
		return "@" + p.Username
	case p.Name != "":
		return p.Name
	default:
		return p.Type + "#" + strconv.FormatInt(p.ID, 10)
	}
}

// describePeer resolves a peer to a peerRef using the entities from a query.
func describePeer(p tg.PeerClass, ent peer.Entities) peerRef {
	switch v := p.(type) {
	case *tg.PeerUser:
		ref := peerRef{ID: v.UserID, Type: peerUser}
		if u, ok := ent.User(v.UserID); ok {
			ref.Name = strings.TrimSpace(u.FirstName + " " + u.LastName)
			ref.Username = u.Username
		}
		return ref
	case *tg.PeerChat:
		ref := peerRef{ID: v.ChatID, Type: peerChat}
		if c, ok := ent.Chat(v.ChatID); ok {
			ref.Name = c.Title
		}
		return ref
	case *tg.PeerChannel:
		ref := peerRef{ID: v.ChannelID, Type: peerChannel}
		if c, ok := ent.Channel(v.ChannelID); ok {
			ref.Name = c.Title
			ref.Username = c.Username
		}
		return ref
	default:
		return peerRef{Type: peerUnknown}
	}
}

// mediaType returns a short lowercase name for a message media, e.g. "photo".
func mediaType(m tg.MessageMediaClass) string {
	if _, ok := m.(*tg.MessageMediaEmpty); ok {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(m.TypeName(), "messageMedia"))
}

// messagePreview returns a one-line preview of a message's text, collapsing
// whitespace and noting media-only messages.
func messagePreview(m tg.MessageClass) string {
	msg, ok := m.(*tg.Message)
	if !ok {
		return ""
	}
	text := strings.Join(strings.Fields(msg.Message), " ")
	if text == "" {
		if _, hasMedia := msg.GetMedia(); hasMedia {
			return "[media]"
		}
	}
	return text
}
