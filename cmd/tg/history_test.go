package main

import (
	"context"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestListHistory(t *testing.T) {
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if _, ok := req.(*tg.MessagesGetHistoryRequest); ok {
			return &tg.MessagesMessages{
				Messages: []tg.MessageClass{
					// API order is newest-first.
					&tg.Message{ID: 2, PeerID: &tg.PeerUser{UserID: 5}, Message: "second", Date: 20, Out: true},
					&tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 5}, Message: "first", Date: 10},
				},
				Users: []tg.UserClass{&tg.User{ID: 5, Username: "alice"}},
			}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	res, err := listHistory(context.Background(), api, &tg.InputPeerSelf{}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}
	// Output should be chronological (oldest-first).
	if res.Messages[0].ID != 1 || res.Messages[1].ID != 2 {
		t.Errorf("order = %d,%d want 1,2", res.Messages[0].ID, res.Messages[1].ID)
	}
	if res.Messages[0].Text != "first" || !res.Messages[1].Out {
		t.Errorf("unexpected content: %+v", res.Messages)
	}
}

func TestListHistoryLimit(t *testing.T) {
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if _, ok := req.(*tg.MessagesGetHistoryRequest); ok {
			return &tg.MessagesMessages{
				Messages: []tg.MessageClass{
					&tg.Message{ID: 3, PeerID: &tg.PeerUser{UserID: 5}, Date: 3},
					&tg.Message{ID: 2, PeerID: &tg.PeerUser{UserID: 5}, Date: 2},
					&tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 5}, Date: 1},
				},
				Users: []tg.UserClass{&tg.User{ID: 5}},
			}, nil
		}
		return nil, errors.Errorf("unexpected request %T", req)
	})

	res, err := listHistory(context.Background(), api, &tg.InputPeerSelf{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("limit not respected: got %d", len(res.Messages))
	}
}

// TestListHistoryBatchSize asserts the per-request limit is batched (capped at
// 100) instead of the iterator default of 1, and never goes below 1 (a
// non-positive batch size panics the iterator).
func TestListHistoryBatchSize(t *testing.T) {
	for _, tt := range []struct {
		limit     int
		wantBatch int
	}{
		{limit: 5, wantBatch: 5},
		{limit: 100, wantBatch: 100},
		{limit: 350, wantBatch: 100},
		{limit: 0, wantBatch: 1},
		{limit: -1, wantBatch: 1},
	} {
		var gotBatch int
		api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
			r, ok := req.(*tg.MessagesGetHistoryRequest)
			if !ok {
				return nil, errors.Errorf("unexpected request %T", req)
			}
			gotBatch = r.Limit
			return &tg.MessagesMessages{
				Messages: []tg.MessageClass{
					&tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 5}, Date: 1},
				},
				Users: []tg.UserClass{&tg.User{ID: 5}},
			}, nil
		})

		if _, err := listHistory(context.Background(), api, &tg.InputPeerSelf{}, tt.limit); err != nil {
			t.Fatalf("limit %d: %v", tt.limit, err)
		}
		if gotBatch != tt.wantBatch {
			t.Errorf("limit %d: request limit = %d, want %d", tt.limit, gotBatch, tt.wantBatch)
		}
	}
}
