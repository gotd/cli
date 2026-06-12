package main

import (
	"context"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

// sentResult is the result of a command that sends a message.
type sentResult struct {
	Peer      string `json:"peer,omitempty"`
	MessageID int    `json:"message_id"`
}

// MarshalText renders a short acknowledgement.
func (r sentResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "sent message #%d\n", r.MessageID)
	return err
}

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

				b, options := msg.apply(builderFor(sender, peer), text)
				id, err := unpack.MessageID(b.StyledText(ctx, options...))
				if err != nil {
					return errors.Wrap(err, "send")
				}
				return a.printer.Emit(sentResult{Peer: peer, MessageID: id})
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
