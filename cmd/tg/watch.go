package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
)

// watchEvent is one streamed message.
type watchEvent struct {
	Peer    peerRef     `json:"peer"`
	Message messageItem `json:"message"`
}

// messageStream turns incoming new-message updates into watchEvents, filtered by
// an optional peer id, delivered to onEvent. Safe for concurrent dispatch.
type messageStream struct {
	filterID int64
	onEvent  func(watchEvent)
	mu       sync.Mutex
}

func (s *messageStream) handle(msg tg.MessageClass, e tg.Entities) {
	m, ok := msg.(*tg.Message)
	if !ok {
		return
	}
	ent := peer.EntitiesFromUpdate(e)
	ev := watchEvent{Peer: describePeer(m.PeerID, ent), Message: buildMessageItem(m, ent)}
	if s.filterID != 0 && ev.Peer.ID != s.filterID {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent(ev)
}

// register wires the stream onto an update dispatcher.
func (s *messageStream) register(d tg.UpdateDispatcher) {
	d.OnNewMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		s.handle(u.Message, e)
		return nil
	})
	d.OnNewChannelMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		s.handle(u.Message, e)
		return nil
	})
}

// resolveFilter resolves an optional peer argument to a filter id.
func (a *app) resolveFilter(ctx context.Context, api *tg.Client, args []string) (int64, error) {
	if len(args) == 0 {
		return 0, nil
	}
	m, err := a.manager(api)
	if err != nil {
		return 0, err
	}
	p, err := m.Resolve(ctx, args[0])
	if err != nil {
		return 0, errors.Wrapf(err, "resolve %q", args[0])
	}
	return p.ID(), nil
}

// emitLine writes one streamed event (JSON line or text line) to stdout.
func emitLine(format output.Format, ev watchEvent) {
	if format == output.JSON {
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintln(os.Stdout, string(b))
		return
	}
	line := ev.Peer.label()
	if ev.Message.Text != "" {
		line += ": " + ev.Message.Text
	} else if ev.Message.Media != "" {
		line += ": [" + ev.Message.Media + "]"
	}
	_, _ = fmt.Fprintln(os.Stdout, line)
}

func (a *app) newWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "watch [peer]",
		Short:             "Stream new messages as they arrive",
		GroupID:           groupMessaging,
		Long:              "Stream incoming messages (optionally for one peer) as JSON lines until interrupted.",
		Example:           "  tg watch\n  tg watch @durov --output json",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.connect(cmd.Context(), runParams{auth: authUser, updates: true},
				func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error {
					if err := requireAuth(ctx, client); err != nil {
						return err
					}
					filterID, err := a.resolveFilter(ctx, client.API(), args)
					if err != nil {
						return err
					}

					format := a.printer.Format()
					stream := &messageStream{
						filterID: filterID,
						onEvent:  func(ev watchEvent) { emitLine(format, ev) },
					}
					stream.register(d)

					_, _ = fmt.Fprintln(os.Stderr, "Watching for new messages (Ctrl-C to stop)…")
					<-ctx.Done()
					return nil
				})
		},
	}
	return cmd
}

// requireAuth returns errNotAuthorized if the session is not logged in.
func requireAuth(ctx context.Context, client *telegram.Client) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return errors.Wrap(err, "auth status")
	}
	if !status.Authorized {
		return errNotAuthorized
	}
	return nil
}
