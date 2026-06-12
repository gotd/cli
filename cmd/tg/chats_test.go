package main

import (
	"context"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func dialogsResult(dialogs []tg.DialogClass, messages []tg.MessageClass, users []tg.UserClass) func(bin.Encoder) (bin.Encoder, error) {
	return func(req bin.Encoder) (bin.Encoder, error) {
		if _, ok := req.(*tg.MessagesGetDialogsRequest); ok {
			return &tg.MessagesDialogs{Dialogs: dialogs, Messages: messages, Users: users}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	}
}

func TestListChats(t *testing.T) {
	api := newFuncAPI(t, dialogsResult(
		[]tg.DialogClass{
			&tg.Dialog{
				Peer:        &tg.PeerUser{UserID: 42},
				TopMessage:  10,
				UnreadCount: 3,
				Pinned:      true,
			},
		},
		[]tg.MessageClass{
			&tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello there", Date: 123},
		},
		[]tg.UserClass{&tg.User{ID: 42, Username: "durov", FirstName: "Pavel"}},
	))

	list, err := listChats(context.Background(), api, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Chats) != 1 {
		t.Fatalf("got %d chats, want 1", len(list.Chats))
	}
	c := list.Chats[0]
	if c.Peer.ID != 42 || c.Peer.Username != "durov" {
		t.Errorf("peer = %+v", c.Peer)
	}
	if c.Unread != 3 || !c.Pinned {
		t.Errorf("unread/pinned = %d/%v", c.Unread, c.Pinned)
	}
	if c.LastMessage != "hello there" {
		t.Errorf("last = %q", c.LastMessage)
	}
}

func TestListChatsLimit(t *testing.T) {
	api := newFuncAPI(t, dialogsResult(
		[]tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerUser{UserID: 1}, TopMessage: 1},
			&tg.Dialog{Peer: &tg.PeerUser{UserID: 2}, TopMessage: 2},
		},
		[]tg.MessageClass{
			&tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 1}, Date: 1},
			&tg.Message{ID: 2, PeerID: &tg.PeerUser{UserID: 2}, Date: 2},
		},
		[]tg.UserClass{&tg.User{ID: 1}, &tg.User{ID: 2}},
	))

	list, err := listChats(context.Background(), api, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Chats) != 1 {
		t.Fatalf("limit not respected: got %d", len(list.Chats))
	}
}
