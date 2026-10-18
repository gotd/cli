package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/peers"
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

// MarshalText renders a peer reference on one line.
func (p peerRef) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "%s\t%s\tid=%d\n", p.Type, p.label(), p.ID)
	return err
}

// describeManagedPeer converts a resolved peers.Peer into a peerRef.
func describeManagedPeer(p peers.Peer) peerRef {
	ref := peerRef{ID: p.ID(), Name: p.VisibleName()}
	if u, ok := p.Username(); ok {
		ref.Username = u
	}
	switch p.(type) {
	case peers.User:
		ref.Type = peerUser
	case peers.Chat:
		ref.Type = peerChat
	case peers.Channel:
		ref.Type = peerChannel
	default:
		ref.Type = peerUnknown
	}
	return ref
}

// peerListResult is a list of peers (search results, etc.).
type peerListResult struct {
	Peers []peerRef `json:"peers"`
}

// MarshalText renders one peer per line.
func (r peerListResult) MarshalText(w io.Writer) error {
	for _, p := range r.Peers {
		if err := p.MarshalText(w); err != nil {
			return err
		}
	}
	return nil
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

// extractChats returns the chats carried by an updates response.
func extractChats(u tg.UpdatesClass) []tg.ChatClass {
	switch v := u.(type) {
	case *tg.Updates:
		return v.Chats
	case *tg.UpdatesCombined:
		return v.Chats
	default:
		return nil
	}
}

// firstChatRef returns a peerRef for the first chat/channel in an updates
// response (e.g. a freshly created group).
func firstChatRef(u tg.UpdatesClass) (peerRef, bool) {
	for _, c := range extractChats(u) {
		switch ch := c.(type) {
		case *tg.Chat:
			return peerRef{ID: ch.ID, Type: peerChat, Name: ch.Title}, true
		case *tg.Channel:
			return peerRef{ID: ch.ID, Type: peerChannel, Name: ch.Title, Username: ch.Username}, true
		}
	}
	return peerRef{}, false
}

// entitiesOf builds peer.Entities from user/chat lists in an RPC response.
func entitiesOf(users []tg.UserClass, chats []tg.ChatClass) peer.Entities {
	return peer.NewEntities(
		tg.UserClassArray(users).UserToMap(),
		tg.ChatClassArray(chats).ChatToMap(),
		tg.ChatClassArray(chats).ChannelToMap(),
	)
}

// messagesFrom extracts the messages and entities from a messages RPC response.
func messagesFrom(res tg.MessagesMessagesClass) ([]tg.MessageClass, peer.Entities, error) {
	mod, ok := res.AsModified()
	if !ok {
		return nil, peer.Entities{}, errors.Errorf("unexpected messages type %T", res)
	}
	return mod.GetMessages(), entitiesOf(mod.GetUsers(), mod.GetChats()), nil
}

// buildMessageItem maps a raw message to a messageItem using entities for names.
func buildMessageItem(msg *tg.Message, ent peer.Entities) messageItem {
	item := messageItem{
		ID:   msg.ID,
		Date: msg.Date,
		Out:  msg.Out,
		Text: msg.Message,
	}
	if media, ok := msg.GetMedia(); ok {
		item.Media = mediaType(media)
	}
	if from, ok := msg.GetFromID(); ok {
		ref := describePeer(from, ent)
		item.From = &ref
	}
	if rt, ok := msg.GetReplyTo(); ok {
		if h, ok := rt.(*tg.MessageReplyHeader); ok {
			item.ReplyTo = h.ReplyToMsgID
		}
	}
	item.EditDate, _ = msg.GetEditDate()
	item.ViaBotID, _ = msg.GetViaBotID()
	item.GroupedID, _ = msg.GetGroupedID()
	if media, ok := msg.GetMedia(); ok {
		item.File = newFileInfo(media)
	}
	if ents, ok := msg.GetEntities(); ok {
		item.Entities = newEntityItems(msg.Message, ents)
	}
	if rm, ok := msg.GetReplyMarkup(); ok {
		item.ReplyMarkup = newReplyMarkup(rm)
	}
	return item
}

// File kinds reported in fileInfo.Kind. The upload --type values share the
// names of the document kinds.
const (
	fileKindFile      = "file"
	fileKindPhoto     = "photo"
	fileKindVideo     = "video"
	fileKindVideoNote = "video_note"
	fileKindAudio     = "audio"
	fileKindVoice     = "voice"
	fileKindAnimation = "animation"
	fileKindSticker   = "sticker"
)

// fileInfo describes a message's document or photo.
type fileInfo struct {
	// Kind refines a document: "file", "video", "video_note", "audio",
	// "voice", "animation" or "sticker"; photos are "photo".
	Kind     string  `json:"kind"`
	Name     string  `json:"name,omitempty"`
	MIME     string  `json:"mime,omitempty"`
	Size     int64   `json:"size,omitempty"`
	Duration float64 `json:"duration,omitempty"` // seconds
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
}

// newFileInfo describes document and photo media; nil for other media.
func newFileInfo(m tg.MessageMediaClass) *fileInfo {
	switch v := m.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := v.Document.AsNotEmpty()
		if !ok {
			return nil
		}
		return documentInfo(doc)
	case *tg.MessageMediaPhoto:
		ph, ok := v.Photo.AsNotEmpty()
		if !ok {
			return nil
		}
		info := &fileInfo{Kind: fileKindPhoto, MIME: "image/jpeg"}
		for _, s := range ph.Sizes {
			if size, ok := s.(*tg.PhotoSize); ok && size.W*size.H >= info.Width*info.Height {
				info.Width, info.Height, info.Size = size.W, size.H, int64(size.Size)
			}
			if size, ok := s.(*tg.PhotoSizeProgressive); ok && size.W*size.H >= info.Width*info.Height {
				info.Width, info.Height = size.W, size.H
				if n := len(size.Sizes); n > 0 {
					info.Size = int64(size.Sizes[n-1])
				}
			}
		}
		return info
	default:
		return nil
	}
}

// documentInfo classifies a document by its attributes.
func documentInfo(doc *tg.Document) *fileInfo {
	info := &fileInfo{Kind: fileKindFile, MIME: doc.MimeType, Size: doc.Size}
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeFilename:
			info.Name = a.FileName
		case *tg.DocumentAttributeVideo:
			info.Kind = fileKindVideo
			if a.RoundMessage {
				info.Kind = fileKindVideoNote
			}
			info.Duration, info.Width, info.Height = a.Duration, a.W, a.H
		case *tg.DocumentAttributeAudio:
			info.Kind = fileKindAudio
			if a.Voice {
				info.Kind = fileKindVoice
			}
			info.Duration = float64(a.Duration)
		case *tg.DocumentAttributeImageSize:
			info.Width, info.Height = a.W, a.H
		}
	}
	// Animated and sticker attributes override video/image classification.
	for _, attr := range doc.Attributes {
		switch attr.(type) {
		case *tg.DocumentAttributeAnimated:
			info.Kind = fileKindAnimation
		case *tg.DocumentAttributeSticker:
			info.Kind = fileKindSticker
		}
	}
	return info
}

// entityItem is one formatting entity of a message text. Offset and Length
// count UTF-16 code units, as Telegram does; Text is the covered substring.
type entityItem struct {
	Type     string `json:"type"`
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	Text     string `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	Language string `json:"language,omitempty"`
}

// newEntityItems converts message entities, e.g. messageEntityTextUrl ->
// {"type":"text_url","url":...}.
func newEntityItems(text string, ents []tg.MessageEntityClass) []entityItem {
	units := utf16.Encode([]rune(text))
	out := make([]entityItem, 0, len(ents))
	for _, e := range ents {
		item := entityItem{
			Type:   snakeCase(strings.TrimPrefix(e.TypeName(), "messageEntity")),
			Offset: e.GetOffset(),
			Length: e.GetLength(),
		}
		if start, end := item.Offset, item.Offset+item.Length; start >= 0 && start <= end && end <= len(units) {
			item.Text = string(utf16.Decode(units[start:end]))
		}
		switch v := e.(type) {
		case *tg.MessageEntityTextURL:
			item.URL = v.URL
		case *tg.MessageEntityMentionName:
			item.UserID = v.UserID
		case *tg.MessageEntityPre:
			item.Language = v.Language
		}
		out = append(out, item)
	}
	return out
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
