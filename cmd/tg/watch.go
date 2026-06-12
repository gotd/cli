package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

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

// resolveFilterFor resolves an optional peer argument to a filter id, using the
// given account's peer cache.
func (a *app) resolveFilterFor(ctx context.Context, api *tg.Client, st *accountState, args []string) (int64, error) {
	if len(args) == 0 {
		return 0, nil
	}
	m, err := a.managerFor(api, st)
	if err != nil {
		return 0, err
	}
	p, err := m.Resolve(ctx, args[0])
	if err != nil {
		return 0, errors.Wrapf(err, "resolve %q", args[0])
	}
	return p.ID(), nil
}

// emitLine writes one streamed event (JSON line or text line) to stdout. When
// account is non-empty (multi-account watch) it is included.
func emitLine(format output.Format, account string, ev watchEvent) {
	if format == output.JSON {
		payload := struct {
			Account string `json:"account,omitempty"`
			watchEvent
		}{Account: account, watchEvent: ev}
		b, err := json.Marshal(payload)
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
	if account != "" {
		line = "[" + account + "] " + line
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
			labels, err := a.selectedLabels()
			if err != nil {
				return err
			}
			if len(labels) > 1 {
				return a.watchAll(cmd.Context(), labels, args)
			}
			return a.watchOne(cmd.Context(), labels[0], "", args)
		},
	}
	return cmd
}

// watchOne streams messages from a single account. The label header (account)
// is non-empty only in multi-account mode.
func (a *app) watchOne(ctx context.Context, label, header string, args []string) error {
	st, err := a.accountState(label)
	if err != nil {
		return err
	}
	return a.watchWith(ctx, st, header, args, nil)
}

// watchAll streams messages from every account concurrently, merged into one
// labeled stream.
func (a *app) watchAll(ctx context.Context, labels, args []string) error {
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	for _, label := range labels {
		st, err := a.accountState(label)
		if err != nil {
			return err
		}
		g.Go(func() error {
			if err := a.watchWith(ctx, st, st.label, args, &mu); err != nil {
				return errors.Wrapf(err, "account %q", st.label)
			}
			return nil
		})
	}
	return g.Wait()
}

// watchWith connects to one account and streams until ctx is done. mu, if set,
// serializes stdout across concurrent accounts.
func (a *app) watchWith(ctx context.Context, st *accountState, header string, args []string, mu *sync.Mutex) error {
	format := a.printer.Format()
	return a.connectWith(ctx, st, runParams{auth: authUser, updates: true},
		func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error {
			if err := requireAuth(ctx, client); err != nil {
				return err
			}
			filterID, err := a.resolveFilterFor(ctx, client.API(), st, args)
			if err != nil {
				return err
			}

			stream := &messageStream{
				filterID: filterID,
				onEvent: func(ev watchEvent) {
					if mu != nil {
						mu.Lock()
						defer mu.Unlock()
					}
					emitLine(format, header, ev)
				},
			}
			stream.register(d)

			_, _ = fmt.Fprintf(os.Stderr, "Watching %s for new messages (Ctrl-C to stop)…\n", st.label)
			<-ctx.Done()
			return nil
		})
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
