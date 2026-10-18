package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

func (a *app) newReplyCmd() *cobra.Command {
	var (
		fromStdin bool
		msg       messageOptions
		wait      waitOptions
	)

	cmd := &cobra.Command{
		Use:     "reply <peer> <message-id> [text]",
		Short:   "Reply to a specific message",
		GroupID: groupMessaging,
		Long: `Send a reply to a specific message in a peer's history. The peer is
me/self, @username, phone, or a t.me link. The text may come from --stdin.
With --wait the peer's response is collected, as for ` + "`tg send`.",
		Example: `  tg reply @durov 12345 "great post"
  tg reply me 1000 "note to self"
  tg reply @mybot 4242 "Berlin" --wait 20s -o json`,
		Args:              cobra.RangeArgs(2, 3),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			peer := args[0]
			replyTo, err := parseMessageID(args[1])
			if err != nil {
				return err
			}
			text, err := messageText(args[2:], fromStdin, cmd.InOrStdin())
			if err != nil {
				return err
			}

			collector := newReplyCollector()
			return a.run(cmd.Context(), wait.runParams(collector), func(ctx context.Context, api *tg.Client) error {
				sender, m, err := a.sender(api)
				if err != nil {
					return err
				}
				bf, err := targetFor(ctx, m, sender, peer, collector, wait)
				if err != nil {
					return err
				}

				b, options := msg.apply(bf, text)
				id, err := unpack.MessageID(b.Reply(replyTo).StyledText(ctx, options...))
				if err != nil {
					return errors.Wrap(err, "reply")
				}
				res := sentResult{Peer: peer, MessageID: id}
				collector.setAction(id, id)
				if res.conversation, err = awaitResponse(ctx, collector, wait); err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&fromStdin, "stdin", false, "read the reply text from standard input")
	msg.register(fs)
	wait.register(fs)

	return cmd
}
