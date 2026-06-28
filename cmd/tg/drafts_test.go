package main

import (
	"context"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// TestListDraftsBatchSize asserts the dialog scan is batched at 100 per request
// instead of the iterator default of 1: listDrafts walks every dialog, so a
// batch size of 1 would issue one messages.getDialogs RPC per dialog.
func TestListDraftsBatchSize(t *testing.T) {
	var gotBatch int
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		r, ok := req.(*tg.MessagesGetDialogsRequest)
		if !ok {
			return nil, errors.Errorf("unexpected request %T", req)
		}
		gotBatch = r.Limit
		return &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{&tg.Dialog{
				Peer:       &tg.PeerUser{UserID: 1},
				TopMessage: 1,
				Draft:      &tg.DraftMessage{Message: "wip"},
			}},
			Messages: []tg.MessageClass{&tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 1}, Date: 1}},
			Users:    []tg.UserClass{&tg.User{ID: 1}},
		}, nil
	})

	res, err := listDrafts(context.Background(), api)
	if err != nil {
		t.Fatal(err)
	}
	if gotBatch != 100 {
		t.Errorf("request limit = %d, want 100", gotBatch)
	}
	if len(res.Drafts) != 1 || res.Drafts[0].Message != "wip" {
		t.Errorf("drafts = %+v", res.Drafts)
	}
}
