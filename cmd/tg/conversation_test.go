package main

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

const testBotID = 7001

// collectorHarness feeds updates into a replyCollector through a real
// dispatcher, the way the client delivers them.
type collectorHarness struct {
	t *testing.T
	d tg.UpdateDispatcher
	c *replyCollector
}

func newCollectorHarness(t *testing.T) *collectorHarness {
	d := tg.NewUpdateDispatcher()
	c := newReplyCollector()
	c.register(d)
	return &collectorHarness{t: t, d: d, c: c}
}

func (h *collectorHarness) push(u tg.UpdateClass) {
	h.t.Helper()
	if err := h.d.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{u}}); err != nil {
		h.t.Fatal(err)
	}
}

func botMessage(id int, text string) *tg.Message {
	return &tg.Message{ID: id, PeerID: &tg.PeerUser{UserID: testBotID}, Message: text, Date: id}
}

func (h *collectorHarness) newMessage(m *tg.Message)  { h.push(&tg.UpdateNewMessage{Message: m}) }
func (h *collectorHarness) editMessage(m *tg.Message) { h.push(&tg.UpdateEditMessage{Message: m}) }

func TestCollectorFiltersAndUpserts(t *testing.T) {
	h := newCollectorHarness(t)

	h.newMessage(botMessage(1, "before arm")) // ignored: not armed yet
	h.c.arm(testBotID)

	h.newMessage(botMessage(10, "hello"))
	h.newMessage(&tg.Message{ID: 11, PeerID: &tg.PeerUser{UserID: 999}, Message: "other chat"})
	h.newMessage(&tg.Message{ID: 12, PeerID: &tg.PeerUser{UserID: testBotID}, Message: "/start", Out: true})

	streamed := botMessage(10, "hello, world") // the bot edits its reply
	streamed.SetEditDate(50)
	h.editMessage(streamed)

	menu := botMessage(5, "menu (page 2)") // an older message edited after a button press
	h.editMessage(menu)

	conv := h.c.snapshot(false)
	if len(conv.Replies) != 1 || conv.Replies[0].ID != 10 || conv.Replies[0].Text != "hello, world" || conv.Replies[0].EditDate != 50 {
		t.Errorf("replies = %+v", conv.Replies)
	}
	if len(conv.Edits) != 1 || conv.Edits[0].ID != 5 {
		t.Errorf("edits = %+v", conv.Edits)
	}
}

func TestCollectorRegisterResets(t *testing.T) {
	h := newCollectorHarness(t)
	h.c.arm(testBotID)
	h.newMessage(botMessage(10, "first account"))

	h.c.register(tg.NewUpdateDispatcher()) // next connection (e.g. --account all)
	if conv := h.c.snapshot(false); len(conv.Replies) != 0 || h.c.peerID != 0 {
		t.Errorf("collector not reset: %+v", conv)
	}
}

func TestCollectSettles(t *testing.T) {
	h := newCollectorHarness(t)
	h.c.arm(testBotID)
	go func() {
		h.newMessage(botMessage(10, "one"))
		h.newMessage(botMessage(11, "two"))
	}()

	start := time.Now()
	conv, err := h.c.collect(context.Background(), waitOptions{wait: 5 * time.Second, settle: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if conv.TimedOut || len(conv.Replies) != 2 {
		t.Errorf("conv = %+v", conv)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("did not settle early: %s", elapsed)
	}
}

func TestCollectReplyCount(t *testing.T) {
	h := newCollectorHarness(t)
	h.c.arm(testBotID)
	go func() {
		h.newMessage(botMessage(10, "one"))
		time.Sleep(50 * time.Millisecond)
		h.newMessage(botMessage(11, "two"))
	}()

	// A settle shorter than the gap would stop after one reply; --replies
	// ignores settle and waits for both.
	conv, err := h.c.collect(context.Background(), waitOptions{wait: 5 * time.Second, settle: time.Millisecond, replies: 2})
	if err != nil {
		t.Fatal(err)
	}
	if conv.TimedOut || len(conv.Replies) != 2 {
		t.Errorf("conv = %+v", conv)
	}
}

func TestCollectTimesOutWithoutReply(t *testing.T) {
	h := newCollectorHarness(t)
	h.c.arm(testBotID)
	conv, err := h.c.collect(context.Background(), waitOptions{wait: 50 * time.Millisecond, settle: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !conv.TimedOut || len(conv.Replies) != 0 || conv.Replies == nil {
		t.Errorf("conv = %+v; want timed out with an empty (non-nil) reply list", conv)
	}
}

func TestCollectTypingKeepsWaiting(t *testing.T) {
	h := newCollectorHarness(t)
	h.c.arm(testBotID)
	h.newMessage(botMessage(10, "thinking…"))
	h.push(&tg.UpdateUserTyping{UserID: testBotID, Action: &tg.SendMessageTypingAction{}})

	// Typing extends the settle period to typingGrace, so a short settle does
	// not end the wait; the overall --wait does.
	start := time.Now()
	conv, err := h.c.collect(context.Background(), waitOptions{wait: 300 * time.Millisecond, settle: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !conv.TimedOut {
		t.Errorf("typing should have kept the wait open; settled after %s", time.Since(start))
	}
}

func TestCollectRespectsContextDeadline(t *testing.T) {
	h := newCollectorHarness(t)
	h.c.arm(testBotID)
	ctx, cancel := context.WithTimeout(context.Background(), deadlineMargin+200*time.Millisecond)
	defer cancel()

	// --wait is longer than --timeout: collect must return before the deadline.
	conv, err := h.c.collect(ctx, waitOptions{wait: time.Minute, settle: time.Second})
	if err != nil {
		t.Fatalf("collect should finish before the deadline: %v", err)
	}
	if !conv.TimedOut || ctx.Err() != nil {
		t.Errorf("conv = %+v, ctx err = %v", conv, ctx.Err())
	}
}

func TestPeerIDOf(t *testing.T) {
	for p, want := range map[tg.PeerClass]int64{
		&tg.PeerUser{UserID: 1}:       1,
		&tg.PeerChat{ChatID: 2}:       2,
		&tg.PeerChannel{ChannelID: 3}: 3,
	} {
		if got := peerIDOf(p); got != want {
			t.Errorf("peerIDOf(%T) = %d, want %d", p, got, want)
		}
	}
}
