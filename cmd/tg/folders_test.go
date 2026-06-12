package main

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestUpdatePeerList(t *testing.T) {
	a := &tg.InputPeerUser{UserID: 1}
	b := &tg.InputPeerChannel{ChannelID: 2}

	// Add b to [a].
	got := updatePeerList([]tg.InputPeerClass{a}, b, true)
	if len(got) != 2 {
		t.Fatalf("add: got %d, want 2", len(got))
	}

	// Adding a again should not duplicate it.
	got = updatePeerList([]tg.InputPeerClass{a, b}, a, true)
	if len(got) != 2 {
		t.Fatalf("add dup: got %d, want 2", len(got))
	}

	// Remove a from [a, b].
	got = updatePeerList([]tg.InputPeerClass{a, b}, a, false)
	if len(got) != 1 {
		t.Fatalf("remove: got %d, want 1", len(got))
	}
	if inputPeerID(got[0]) != 2 {
		t.Errorf("remaining peer id = %d, want 2", inputPeerID(got[0]))
	}
}

func TestInputPeerID(t *testing.T) {
	if inputPeerID(&tg.InputPeerUser{UserID: 5}) != 5 {
		t.Error("user id mismatch")
	}
	if inputPeerID(&tg.InputPeerSelf{}) != -1 {
		t.Error("self id should be -1")
	}
}
