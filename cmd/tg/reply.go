package main

import (
	"context"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

func (a *app) newReplyCmd() *cobra.Command {
	var msg messageOptions

	cmd := &cobra.Command{
		Use:     "reply <peer> <message-id> <text>",
		Short:   "Reply to a specific message",
		GroupID: groupMessaging,
		Long: `Send a reply to a specific message in a peer's history. The peer is
me/self, @username, phone, or a t.me link.`,
		Example: `  tg reply @durov 12345 "great post"
  tg reply me 1000 "note to self"`,
		Args:              cobra.ExactArgs(3),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			peer := args[0]
			replyTo, err := strconv.Atoi(args[1])
			if err != nil {
				return errors.Wrap(err, "message-id must be an integer")
			}
			text := args[2]

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				sender, err := a.sender(api, authUser)
				if err != nil {
					return err
				}

				b, options := msg.apply(builderFor(sender, peer), text)
				id, err := unpack.MessageID(b.Reply(replyTo).StyledText(ctx, options...))
				if err != nil {
					return errors.Wrap(err, "reply")
				}
				return a.printer.Emit(sentResult{Peer: peer, MessageID: id})
			})
		},
	}

	msg.register(cmd.Flags())

	return cmd
}
