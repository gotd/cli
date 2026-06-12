package main

import (
	"context"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestSearchMessages(t *testing.T) {
	var gotFilter tg.MessagesFilterClass
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if r, ok := req.(*tg.MessagesSearchRequest); ok {
			gotFilter = r.Filter
			return &tg.MessagesMessages{
				Messages: []tg.MessageClass{
					&tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 9}, Message: "found it", Date: 5},
				},
				Users: []tg.UserClass{&tg.User{ID: 9, Username: "bob"}},
			}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	res, err := searchMessages(context.Background(), api, &tg.InputPeerUser{UserID: 9}, "found", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Text != "found it" {
		t.Errorf("unexpected messages: %+v", res.Messages)
	}
	if _, ok := gotFilter.(*tg.InputMessagesFilterEmpty); !ok {
		t.Errorf("default filter = %T, want InputMessagesFilterEmpty", gotFilter)
	}
}

func TestPinnedUsesPinnedFilter(t *testing.T) {
	var gotFilter tg.MessagesFilterClass
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if r, ok := req.(*tg.MessagesSearchRequest); ok {
			gotFilter = r.Filter
			return &tg.MessagesMessages{}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	if _, err := searchMessages(context.Background(), api, &tg.InputPeerUser{UserID: 1}, "", &tg.InputMessagesFilterPinned{}, 10); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotFilter.(*tg.InputMessagesFilterPinned); !ok {
		t.Errorf("filter = %T, want InputMessagesFilterPinned", gotFilter)
	}
}
