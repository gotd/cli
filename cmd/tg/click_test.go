package main

import (
	"context"
	"strings"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func botPeer() *tg.InputPeerUser { return &tg.InputPeerUser{UserID: testBotID, AccessHash: 1} }

func TestPressCallbackButton(t *testing.T) {
	var got *tg.MessagesGetBotCallbackAnswerRequest
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		r, ok := req.(*tg.MessagesGetBotCallbackAnswerRequest)
		if !ok {
			return nil, errors.Errorf("unexpected request %T", req)
		}
		got = r
		return &tg.MessagesBotCallbackAnswer{Message: "Saved", Alert: true}, nil
	})
	btn := keyboardButtons(testInlineMarkup())[0] // "📋 Menu", data "menu"

	res, err := pressButton(context.Background(), api, message.NewSender(api).To(botPeer()), botPeer(), 42, btn)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.MsgID != 42 || string(got.Data) != "menu" || got.Game {
		t.Errorf("request = %+v", got)
	}
	if res.Answer == nil || !res.Answer.Answered || res.Answer.Message != "Saved" || !res.Answer.Alert {
		t.Errorf("answer = %+v", res.Answer)
	}
	if res.Row != 1 || res.Col != 1 || res.MessageID != 42 || res.Button.Data != "menu" {
		t.Errorf("result = %+v", res)
	}
}

func TestPressCallbackButtonUnanswered(t *testing.T) {
	api := newFuncAPI(t, func(bin.Encoder) (bin.Encoder, error) {
		return nil, tgerr.New(400, "BOT_RESPONSE_TIMEOUT")
	})
	btn := keyboardButtons(testInlineMarkup())[0]
	res, err := pressButton(context.Background(), api, message.NewSender(api).To(botPeer()), botPeer(), 42, btn)
	if err != nil {
		t.Fatalf("an unanswered callback is not an error: %v", err)
	}
	if res.Answer == nil || res.Answer.Answered {
		t.Errorf("answer = %+v, want answered=false", res.Answer)
	}
}

func TestPressButtonWithoutRPC(t *testing.T) {
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		return nil, errors.Errorf("no RPC expected, got %T", req)
	})
	docs := keyboardButtons(testInlineMarkup())[2]
	res, err := pressButton(context.Background(), api, message.NewSender(api).To(botPeer()), botPeer(), 42, docs)
	if err != nil || res.Button.URL != "https://example.com" {
		t.Errorf("url button: %+v, %v", res, err)
	}

	share := keyboardButtons(testInlineMarkup())[3]
	if _, err := pressButton(context.Background(), api, message.NewSender(api).To(botPeer()), botPeer(), 42, share); err == nil {
		t.Error("switch_inline buttons should be reported as unsupported")
	}

	locked := keyboardButtons(&tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonCallback{Text: "Transfer", Data: []byte("x"), RequiresPassword: true},
	}}}})[0]
	if _, err := pressButton(context.Background(), api, message.NewSender(api).To(botPeer()), botPeer(), 42, locked); err == nil {
		t.Error("password-protected buttons should be refused")
	}
}

func TestPressReplyKeyboardButtonSendsText(t *testing.T) {
	var sent *tg.MessagesSendMessageRequest
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		r, ok := req.(*tg.MessagesSendMessageRequest)
		if !ok {
			return nil, errors.Errorf("unexpected request %T", req)
		}
		sent = r
		return &tg.UpdateShortSentMessage{ID: 77}, nil
	})
	btn := keyboardButtons(&tg.ReplyKeyboardMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButton{Text: "Yes"},
	}}}})[0]

	res, err := pressButton(context.Background(), api, message.NewSender(api).To(botPeer()), botPeer(), 42, btn)
	if err != nil {
		t.Fatal(err)
	}
	if sent == nil || sent.Message != "Yes" || sent.ReplyTo != nil {
		t.Errorf("sent = %+v (private chats send without reply)", sent)
	}
	if res.SentMessageID != 77 {
		t.Errorf("sent message id = %d", res.SentMessageID)
	}
}

func TestKeyboardMessageLatest(t *testing.T) {
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if _, ok := req.(*tg.MessagesGetHistoryRequest); !ok {
			return nil, errors.Errorf("unexpected request %T", req)
		}
		withButtons := botMessage(8, "pick")
		withButtons.SetReplyMarkup(testInlineMarkup())
		olderButtons := botMessage(3, "old")
		olderButtons.SetReplyMarkup(testInlineMarkup())
		ours := &tg.Message{ID: 9, PeerID: &tg.PeerUser{UserID: testBotID}, Out: true}
		return &tg.MessagesMessages{Messages: []tg.MessageClass{ // newest first
			ours, botMessage(9, "no buttons"), withButtons, olderButtons,
		}}, nil
	})
	msg, err := keyboardMessage(context.Background(), api, botPeer(), 0)
	if err != nil || msg.ID != 8 {
		t.Fatalf("keyboardMessage = %+v, %v; want #8", msg, err)
	}
}

func TestGetMessageScopedToChat(t *testing.T) {
	// messages.getMessages is not scoped to a chat; a message from another
	// private chat must not be returned.
	api := newFuncAPI(t, func(bin.Encoder) (bin.Encoder, error) {
		return &tg.MessagesMessages{Messages: []tg.MessageClass{
			&tg.Message{ID: 5, PeerID: &tg.PeerUser{UserID: 999}},
		}}, nil
	})
	_, err := getMessage(context.Background(), api, botPeer(), 5)
	var nf notFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want not-found", err)
	}
	if _, err := getMessage(context.Background(), api, &tg.InputPeerUser{UserID: 999}, 5); err != nil {
		t.Errorf("same chat: %v", err)
	}
}

func TestParseBotLink(t *testing.T) {
	cases := []struct {
		in, bot, param string
		ok             bool
	}{
		{"https://t.me/mybot?start=ref_42", "@mybot", "ref_42", true},
		{"t.me/mybot", "@mybot", "", true},
		{"tg://resolve?domain=mybot&start=abc", "@mybot", "abc", true},
		{"@mybot", "", "", false},
		{"https://example.com/mybot?start=x", "", "", false},
		{"https://t.me/c/123/4", "", "", false},
	}
	for _, c := range cases {
		bot, param, ok := parseBotLink(c.in)
		if bot != c.bot || param != c.param || ok != c.ok {
			t.Errorf("parseBotLink(%q) = %q, %q, %v", c.in, bot, param, ok)
		}
	}

	if _, _, err := startArgs([]string{"https://t.me/mybot?start=a", "b"}); err == nil {
		t.Error("expected an error for a parameter given twice")
	}
	if bot, param, err := startArgs([]string{"@mybot", "p"}); err != nil || bot != "@mybot" || param != "p" {
		t.Errorf("startArgs = %q, %q, %v", bot, param, err)
	}
}

func TestNewBotInfoResult(t *testing.T) {
	u := &tg.User{ID: testBotID, Bot: true, BotNochats: true, BotInlinePlaceholder: "Search…"}
	full := &tg.UserFull{About: "Bot about"}
	full.SetBotInfo(tg.BotInfo{
		Description: "What I do",
		Commands:    []tg.BotCommand{{Command: "start", Description: "Begin"}, {Command: "help", Description: "Help"}},
		MenuButton:  &tg.BotMenuButtonCommands{},
	})
	res := newBotInfoResult(peerRef{ID: testBotID, Type: peerUser, Username: "mybot"}, u, full)
	if res.About != "Bot about" || res.Description != "What I do" || len(res.Commands) != 2 || res.Commands[1].Command != "help" {
		t.Errorf("result = %+v", res)
	}
	if res.CanJoinGroups || res.InlinePlaceholder != "Search…" || res.MenuButton == nil || res.MenuButton.Type != "commands" {
		t.Errorf("flags = %+v", res)
	}
	var sb strings.Builder
	if err := res.MarshalText(&sb); err != nil || !strings.Contains(sb.String(), "/help - Help") {
		t.Errorf("text = %q, %v", sb.String(), err)
	}

	// A bot without bot info still reports an empty (non-nil) command list.
	if res := newBotInfoResult(peerRef{}, u, &tg.UserFull{}); res.Commands == nil {
		t.Error("commands should be an empty list, not null")
	}
}

func TestMessageText(t *testing.T) {
	if got, err := messageText([]string{"hi"}, false, nil); err != nil || got != "hi" {
		t.Errorf("arg: %q, %v", got, err)
	}
	if got, err := messageText(nil, true, strings.NewReader("secret-code\n")); err != nil || got != "secret-code" {
		t.Errorf("stdin: %q, %v", got, err)
	}
	if got, err := messageText(nil, true, strings.NewReader("line 1\nline 2\r\n")); err != nil || got != "line 1\nline 2" {
		t.Errorf("multi-line stdin: %q, %v", got, err)
	}
	for name, run := range map[string]func() error{
		"missing":    func() error { _, err := messageText(nil, false, nil); return err },
		"both":       func() error { _, err := messageText([]string{"x"}, true, strings.NewReader("y")); return err },
		"empty":      func() error { _, err := messageText(nil, true, strings.NewReader("\n")); return err },
		"oversized":  func() error { _, err := messageText(nil, true, strings.NewReader(strings.Repeat("a", maxStdinText+1))); return err },
		"empty argv": func() error { _, err := messageText([]string{""}, false, nil); return err },
	} {
		var u usageError
		if err := run(); !errors.As(err, &u) {
			t.Errorf("%s: err = %v, want usage error", name, err)
		}
	}
}

func TestListHistoryAfter(t *testing.T) {
	api := newFuncAPI(t, func(req bin.Encoder) (bin.Encoder, error) {
		if _, ok := req.(*tg.MessagesGetHistoryRequest); !ok {
			return nil, errors.Errorf("unexpected request %T", req)
		}
		return &tg.MessagesMessages{Messages: []tg.MessageClass{
			botMessage(12, "c"), botMessage(11, "b"), botMessage(10, "a"), botMessage(9, "old"),
		}}, nil
	})
	res, err := listHistory(context.Background(), api, botPeer(), 30, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 2 || res.Messages[0].ID != 11 || res.Messages[1].ID != 12 {
		t.Errorf("messages = %+v, want #11 and #12", res.Messages)
	}
}
