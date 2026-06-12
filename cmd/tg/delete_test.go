package main

import (
	"context"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestParseIDs(t *testing.T) {
	ids, err := parseIDs([]string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Errorf("ids = %v", ids)
	}
	if _, err := parseIDs([]string{"x"}); err == nil {
		t.Error("expected error for non-numeric id")
	}
}

func TestDeleteMessagesUser(t *testing.T) {
	var msgReq *tg.MessagesDeleteMessagesRequest
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if r, ok := req.(*tg.MessagesDeleteMessagesRequest); ok {
			msgReq = r
			return &tg.MessagesAffectedMessages{}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	err := deleteMessages(context.Background(), api, &tg.InputPeerUser{UserID: 1}, []int{5, 6}, true)
	if err != nil {
		t.Fatal(err)
	}
	if msgReq == nil || !msgReq.Revoke || len(msgReq.ID) != 2 {
		t.Errorf("unexpected request: %+v", msgReq)
	}
}

func TestDeleteMessagesChannel(t *testing.T) {
	var chReq *tg.ChannelsDeleteMessagesRequest
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if r, ok := req.(*tg.ChannelsDeleteMessagesRequest); ok {
			chReq = r
			return &tg.MessagesAffectedMessages{}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	err := deleteMessages(context.Background(), api, &tg.InputPeerChannel{ChannelID: 10, AccessHash: 20}, []int{1}, true)
	if err != nil {
		t.Fatal(err)
	}
	ch, ok := chReq.GetChannel().(*tg.InputChannel)
	if chReq == nil || !ok || ch.ChannelID != 10 {
		t.Errorf("unexpected request: %+v", chReq)
	}
}
