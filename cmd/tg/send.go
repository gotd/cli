package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

func (a *app) newSendCmd() *cobra.Command {
	var (
		peer string
		msg  messageOptions
	)

	cmd := &cobra.Command{
		Use:     "send [flags] [text]",
		Short:   "Send a message to a peer",
		GroupID: groupMessaging,
		Long: `Send a text message. With no --peer, the message goes to your own
Saved Messages, which is handy for notes and agent self-messaging.`,
		Example: `  # Message yourself (Saved Messages)
  tg send "hello"

  # Message a channel or user
  tg send --peer @durov "hi there"

  # HTML formatting, sent silently
  tg send --peer @durov --html --silent "<b>bold</b>"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if len(args) > 0 {
				text = args[0]
			}

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				sender, err := a.sender(api, authUser)
				if err != nil {
					return err
				}

				builder := sender.Self()
				if peer != "" {
					builder = sender.Resolve(peer)
				}

				b, options := msg.apply(builder, text)
				if _, err := b.StyledText(ctx, options...); err != nil {
					return errors.Wrap(err, "send")
				}

				return nil
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&peer, "peer", "p", "",
		"peer to write (channel name or username, phone number or deep link); default: yourself")
	msg.register(fs)
	registerPeerCompletion(cmd, "peer")

	return cmd
}
