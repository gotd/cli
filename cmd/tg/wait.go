package main

import (
	"context"
	"io"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func (a *app) newWaitCmd() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:               "wait [peer]",
		Short:             "Block until a new message arrives",
		GroupID:           groupMessaging,
		Long:              "Block until the next incoming message (optionally from one peer), then print it and exit.",
		Example:           "  tg wait @durov --timeout 5m\n  tg wait --output json",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.connect(cmd.Context(), runParams{auth: authUser, updates: true},
				func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error {
					if err := requireAuth(ctx, client); err != nil {
						return err
					}
					filterID, err := a.resolveFilterFor(ctx, client.API(), a.active, args)
					if err != nil {
						return err
					}

					events := make(chan watchEvent, 1)
					stream := &messageStream{
						filterID: filterID,
						onEvent: func(ev watchEvent) {
							select {
							case events <- ev:
							default:
							}
						},
					}
					stream.register(d)

					var timer <-chan time.Time
					if timeout > 0 {
						t := time.NewTimer(timeout)
						defer t.Stop()
						timer = t.C
					}

					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-timer:
						return errors.New("timeout waiting for message")
					case ev := <-events:
						return a.printer.Emit(ev)
					}
				})
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 0, "give up after this duration (0 = wait forever)")

	return cmd
}

// MarshalText renders a single watch event for `tg wait` text output.
func (e watchEvent) MarshalText(w io.Writer) error {
	line := e.Peer.label()
	if e.Message.Text != "" {
		line += ": " + e.Message.Text
	}
	_, err := w.Write([]byte(line + "\n"))
	return err
}
