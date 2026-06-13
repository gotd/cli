package main

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestInviteLinkFromFull(t *testing.T) {
	t.Run("channel with link", func(t *testing.T) {
		full := &tg.MessagesChatFull{FullChat: &tg.ChannelFull{}}
		full.FullChat.(*tg.ChannelFull).SetExportedInvite(&tg.ChatInviteExported{Link: "https://t.me/+abc"})
		got, err := inviteLinkFromFull(full)
		if err != nil {
			t.Fatal(err)
		}
		if got != "https://t.me/+abc" {
			t.Errorf("link = %q", got)
		}
	})

	t.Run("basic chat with link", func(t *testing.T) {
		full := &tg.MessagesChatFull{FullChat: &tg.ChatFull{}}
		full.FullChat.(*tg.ChatFull).SetExportedInvite(&tg.ChatInviteExported{Link: "https://t.me/+xyz"})
		got, err := inviteLinkFromFull(full)
		if err != nil {
			t.Fatal(err)
		}
		if got != "https://t.me/+xyz" {
			t.Errorf("link = %q", got)
		}
	})

	t.Run("no link", func(t *testing.T) {
		full := &tg.MessagesChatFull{FullChat: &tg.ChannelFull{}}
		got, err := inviteLinkFromFull(full)
		if err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Errorf("expected empty link, got %q", got)
		}
	})
}
