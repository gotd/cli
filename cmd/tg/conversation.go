package main

import (
	"context"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/pflag"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
)

// typingGrace is how long a "typing…" action from the peer keeps a
// conversation open: Telegram shows the action for about five seconds and bots
// repeat it while they work.
const typingGrace = 6 * time.Second

// deadlineMargin is kept free before a --timeout deadline so a waiting command
// can still print what it collected.
const deadlineMargin = time.Second

// waitOptions configure collecting a peer's responses to an action (`--wait`).
type waitOptions struct {
	wait    time.Duration
	settle  time.Duration
	replies int
}

// register binds the --wait/--settle/--replies flags.
func (o *waitOptions) register(fs *pflag.FlagSet) {
	fs.DurationVar(&o.wait, "wait", 0,
		"then collect the peer's replies and edits for up to this long (e.g. 30s; 0 = don't wait)")
	fs.DurationVar(&o.settle, "settle", 2*time.Second,
		"with --wait: stop once the peer has been quiet this long after responding")
	fs.IntVar(&o.replies, "replies", 0,
		"with --wait: stop as soon as this many new messages arrived (ignores --settle)")
}

func (o waitOptions) enabled() bool { return o.wait > 0 }

// runParams returns the parameters for a user command that may wait: with
// --wait, updates are enabled and c listens on them.
func (o waitOptions) runParams(c *replyCollector) runParams {
	rp := runParams{auth: authUser}
	if o.enabled() {
		rp.updates = true
		rp.register = c.register
	}
	return rp
}

// awaitResponse collects the peer's response when --wait is set; it returns
// nil otherwise.
func awaitResponse(ctx context.Context, c *replyCollector, o waitOptions) (*conversation, error) {
	if !o.enabled() {
		return nil, nil
	}
	conv, err := c.collect(ctx, o)
	if err != nil {
		return nil, errors.Wrap(err, "stopped waiting for a response")
	}
	return conv, nil
}

// conversation is what the peer did while tg waited after an action, oldest
// first, with messages in their latest version.
//
// Telegram does not say which action a bot message answers, so attribution is
// split by what can be proven:
//   - Replies are the peer's messages ordered after the action's own message:
//     observed activity, very likely the response, but a bot that also sends
//     unprompted messages (or a second conversation with the same bot from this
//     account) can add messages here too.
//   - Linked lists the ids Telegram itself ties to the action: replies that
//     reply_to the sent message and edits of the message whose button was
//     pressed. Those are deterministic.
//   - Earlier holds messages the peer sent before the action's message (ids are
//     ordered per chat), e.g. a late answer to a previous command: never a
//     response to this action.
type conversation struct {
	Replies []messageItem `json:"replies"`
	Edits   []messageItem `json:"edits,omitempty"`
	Linked  []int         `json:"linked,omitempty"`
	Earlier []messageItem `json:"earlier,omitempty"`
	// TimedOut is set when --wait elapsed before the peer settled (or, with
	// --replies, before enough replies arrived).
	TimedOut bool `json:"timed_out,omitempty"`
}

// marshalText renders the collected messages like `tg history`.
func (c *conversation) marshalText(w io.Writer) error {
	if c == nil {
		return nil
	}
	if err := (historyResult{Messages: c.Replies}).MarshalText(w); err != nil {
		return err
	}
	if len(c.Edits) > 0 {
		if _, err := io.WriteString(w, "edited:\n"); err != nil {
			return err
		}
		if err := (historyResult{Messages: c.Edits}).MarshalText(w); err != nil {
			return err
		}
	}
	if len(c.Linked) > 0 {
		ids := make([]string, 0, len(c.Linked))
		for _, id := range c.Linked {
			ids = append(ids, "#"+strconv.Itoa(id))
		}
		if _, err := io.WriteString(w, "linked to the action: "+strings.Join(ids, ", ")+"\n"); err != nil {
			return err
		}
	}
	if len(c.Earlier) > 0 {
		if _, err := io.WriteString(w, "sent before the action (not responses):\n"); err != nil {
			return err
		}
		if err := (historyResult{Messages: c.Earlier}).MarshalText(w); err != nil {
			return err
		}
	}
	if c.TimedOut {
		_, err := io.WriteString(w, "(timed out waiting for the peer)\n")
		return err
	}
	return nil
}

// replyCollector gathers a peer's new messages and edits. Its handlers are
// registered on the dispatcher before the client connects (the dispatcher is
// not safe for concurrent registration) and ignore everything until arm names
// the peer to listen to.
type replyCollector struct {
	mu      sync.Mutex
	peerID  int64 // 0 until armed
	replies []messageItem
	edits   []messageItem
	// after is the action's own message id: the peer's messages with lower
	// ids were sent before it (0 = no such message, e.g. a callback press).
	after int
	// links are the message ids a response can reply to or edit.
	links map[int]bool
	// activity is signaled (non-blocking) on every relevant event.
	activity chan event
}

// event is a kind of activity seen by a replyCollector.
type event int

const (
	eventMessage event = iota // a reply or an edit was collected
	eventTyping               // the peer is typing
)

func newReplyCollector() *replyCollector {
	return &replyCollector{activity: make(chan event, 16)}
}

// register wires the collector onto d and resets it, so one collector can serve
// sequential connections (e.g. --account all).
func (c *replyCollector) register(d tg.UpdateDispatcher) {
	c.mu.Lock()
	c.peerID, c.replies, c.edits, c.after, c.links = 0, nil, nil, 0, nil
	c.mu.Unlock()

	d.OnNewMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		c.onMessage(u.Message, e, false)
		return nil
	})
	d.OnNewChannelMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		c.onMessage(u.Message, e, false)
		return nil
	})
	d.OnEditMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateEditMessage) error {
		c.onMessage(u.Message, e, true)
		return nil
	})
	d.OnEditChannelMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateEditChannelMessage) error {
		c.onMessage(u.Message, e, true)
		return nil
	})
	d.OnUserTyping(func(_ context.Context, _ tg.Entities, u *tg.UpdateUserTyping) error {
		c.onTyping(u.UserID, u.Action)
		return nil
	})
}

// arm starts collecting messages in the chat with peerID. Call it before the
// action, so a response that arrives before the action's RPC returns is kept.
func (c *replyCollector) arm(peerID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peerID = peerID
}

// setAction records the action the collected messages respond to, once its
// RPC returned: after is the action's own message id (0 if it created none) and
// links are the messages a response may reference or edit.
func (c *replyCollector) setAction(after int, links ...int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.after = after
	c.links = make(map[int]bool, len(links))
	for _, id := range links {
		if id != 0 {
			c.links[id] = true
		}
	}
}

func (c *replyCollector) onMessage(msg tg.MessageClass, e tg.Entities, edited bool) {
	m, ok := msg.(*tg.Message)
	// Our own messages (and edits of them) are not responses.
	if !ok || m.Out {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.peerID == 0 || peerIDOf(m.PeerID) != c.peerID {
		return
	}
	item := buildMessageItem(m, peer.EntitiesFromUpdate(e))
	switch {
	case upsertMessage(c.replies, item):
		// A reply edited again (e.g. a bot streaming its answer): keep the
		// latest version in place.
	case edited:
		if !upsertMessage(c.edits, item) {
			c.edits = append(c.edits, item)
		}
	default:
		c.replies = append(c.replies, item)
	}
	c.signal(eventMessage)
}

func (c *replyCollector) onTyping(userID int64, action tg.SendMessageActionClass) {
	if _, cancel := action.(*tg.SendMessageCancelAction); cancel {
		return
	}
	c.mu.Lock()
	armed := c.peerID != 0 && c.peerID == userID
	c.mu.Unlock()
	if armed {
		c.signal(eventTyping)
	}
}

func (c *replyCollector) signal(ev event) {
	select {
	case c.activity <- ev:
	default: // the waiter is already behind; it re-reads counts anyway
	}
}

// upsertMessage replaces the message with item's id in msgs, reporting whether
// one was found.
func upsertMessage(msgs []messageItem, item messageItem) bool {
	for i := range msgs {
		if msgs[i].ID == item.ID {
			msgs[i] = item
			return true
		}
	}
	return false
}

// counts returns how many replies (sent after the action) and edits were
// collected so far.
func (c *replyCollector) counts() (replies, edits int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.replies {
		if m.ID > c.after {
			replies++
		}
	}
	return replies, len(c.edits)
}

func (c *replyCollector) snapshot(timedOut bool) *conversation {
	c.mu.Lock()
	defer c.mu.Unlock()
	conv := &conversation{Replies: []messageItem{}, TimedOut: timedOut}
	for _, m := range c.replies {
		if m.ID > c.after {
			conv.Replies = append(conv.Replies, m)
		} else {
			conv.Earlier = append(conv.Earlier, m)
		}
	}
	conv.Edits = append(conv.Edits, c.edits...)
	for _, msgs := range [][]messageItem{conv.Replies, conv.Edits, conv.Earlier} {
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })
	}
	for _, m := range conv.Replies {
		if c.links[m.ReplyTo] {
			conv.Linked = append(conv.Linked, m.ID)
		}
	}
	for _, m := range conv.Edits {
		if c.links[m.ID] || c.links[m.ReplyTo] {
			conv.Linked = append(conv.Linked, m.ID)
		}
	}
	sort.Ints(conv.Linked)
	return conv
}

// collect blocks until the peer's response is complete and returns it. The
// response is complete when o.replies messages arrived (if set), or else when
// the peer was quiet for o.settle after its first reply or edit; o.wait bounds
// the whole wait. An earlier context deadline (--timeout) shortens the wait so
// the result can still be printed.
func (c *replyCollector) collect(ctx context.Context, o waitOptions) (*conversation, error) {
	limit := o.wait
	if dl, ok := ctx.Deadline(); ok {
		limit = min(limit, max(time.Until(dl)-deadlineMargin, 0))
	}
	deadline := time.NewTimer(limit)
	defer deadline.Stop()

	// settle runs only once something was collected; nil channel otherwise.
	var (
		settle   *time.Timer
		settleCh <-chan time.Time
	)
	defer func() {
		if settle != nil {
			settle.Stop()
		}
	}()
	resetSettle := func(d time.Duration) {
		if settle == nil {
			settle = time.NewTimer(d)
			settleCh = settle.C
			return
		}
		settle.Reset(d)
	}

	for {
		select {
		case <-ctx.Done():
			return c.snapshot(true), ctx.Err()
		case <-deadline.C:
			return c.snapshot(true), nil
		case <-settleCh:
			return c.snapshot(false), nil
		case ev := <-c.activity:
			replies, edits := c.counts()
			if o.replies > 0 {
				if replies >= o.replies {
					return c.snapshot(false), nil
				}
				continue
			}
			if replies+edits == 0 {
				continue // typing before any response: keep waiting
			}
			if ev == eventTyping {
				resetSettle(max(o.settle, typingGrace))
			} else {
				resetSettle(o.settle)
			}
		}
	}
}

// peerIDOf returns the bare id of a peer.
func peerIDOf(p tg.PeerClass) int64 {
	switch v := p.(type) {
	case *tg.PeerUser:
		return v.UserID
	case *tg.PeerChat:
		return v.ChatID
	case *tg.PeerChannel:
		return v.ChannelID
	default:
		return 0
	}
}
