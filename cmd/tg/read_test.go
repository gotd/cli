package main

import (
	"context"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestMarkReadUser(t *testing.T) {
	var gotMessages, gotChannels bool
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		switch req.(type) {
		case *tg.MessagesReadHistoryRequest:
			gotMessages = true
			return &tg.MessagesAffectedMessages{}, nil
		case *tg.ChannelsReadHistoryRequest:
			gotChannels = true
			return &tg.BoolTrue{}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	if err := markRead(context.Background(), api, &tg.InputPeerUser{UserID: 1, AccessHash: 2}); err != nil {
		t.Fatal(err)
	}
	if !gotMessages || gotChannels {
		t.Errorf("expected messages.readHistory, got messages=%v channels=%v", gotMessages, gotChannels)
	}
}

func TestMarkReadChannel(t *testing.T) {
	var gotChannels bool
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		switch req.(type) {
		case *tg.ChannelsReadHistoryRequest:
			gotChannels = true
			return &tg.BoolTrue{}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	if err := markRead(context.Background(), api, &tg.InputPeerChannel{ChannelID: 10, AccessHash: 20}); err != nil {
		t.Fatal(err)
	}
	if !gotChannels {
		t.Error("expected channels.readHistory for channel peer")
	}
}
