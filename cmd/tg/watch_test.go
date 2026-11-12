package main

import (
	"context"
	"sync"
	"testing"

	"github.com/gotd/td/tg"
)

func pushNewMessage(t *testing.T, d tg.UpdateDispatcher, peerID int64, id int) {
	t.Helper()
	upd := &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewMessage{
		Message: &tg.Message{ID: id, PeerID: &tg.PeerUser{UserID: peerID}, Message: "hi"},
	}}}
	if err := d.Handle(context.Background(), upd); err != nil {
		t.Error(err)
	}
}

func TestMessageStreamArm(t *testing.T) {
	var (
		mu  sync.Mutex
		got []int
	)
	s := &messageStream{onEvent: func(ev watchEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev.Message.ID)
	}}
	d := tg.NewUpdateDispatcher()
	s.register(d) // registered before any update is dispatched

	pushNewMessage(t, d, 7, 1) // dropped: not armed yet
	s.arm(7)
	pushNewMessage(t, d, 7, 2)
	pushNewMessage(t, d, 8, 3) // dropped: other peer
	s.arm(0)
	pushNewMessage(t, d, 8, 4)

	if len(got) != 2 || got[0] != 2 || got[1] != 4 {
		t.Errorf("delivered %v, want [2 4]", got)
	}
}

func TestMessageStreamConcurrentArm(t *testing.T) {
	// Updates are dispatched from the client's goroutines while the command
	// arms the stream; with -race this fails if the two are not synchronized.
	s := &messageStream{onEvent: func(watchEvent) {}}
	d := tg.NewUpdateDispatcher()
	s.register(d)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			pushNewMessage(t, d, 7, i)
		}
	}()
	s.arm(7)
	wg.Wait()
}
