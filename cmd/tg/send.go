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

// sentResult is the result of a command that sends a message. With --wait it
// also carries the peer's response.
type sentResult struct {
	Peer      string `json:"peer,omitempty"`
	MessageID int    `json:"message_id"`
	*conversation
}

// MarshalText renders a short acknowledgement and any collected response.
func (r sentResult) MarshalText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "sent message #%d\n", r.MessageID); err != nil {
		return err
	}
	return r.marshalText(w)
}

func (a *app) newSendCmd() *cobra.Command {
	var (
		peer      string
		fromStdin bool
		msg       messageOptions
		wait      waitOptions
	)

	cmd := &cobra.Command{
		Use:     "send [flags] [text]",
		Short:   "Send a message to a peer",
		GroupID: groupMessaging,
		Long: `Send a text message. With no --peer, the message goes to your own
Saved Messages, which is handy for notes and agent self-messaging.

A peer with no username or phone can be addressed by its numeric id with the
"id:" prefix (the "id" field in ` + "`tg chats list --output json`" + `), provided
it has been cached first by running ` + "`tg chats list`" + `.

With --wait, tg listens for the peer's response before sending and returns the
replies (and edits) it collected: it stops when the peer has been quiet for
--settle after responding, when --replies messages arrived, or at --wait.`,
		Example: `  # Message yourself (Saved Messages)
  tg send "hello"

  # Message a channel or user
  tg send --peer @durov "hi there"

  # Message a peer without a username, by numeric id (cache it first)
  tg chats list >/dev/null && tg send --peer id:2201861038 "hi"

  # HTML formatting, sent silently
  tg send --peer @durov --html --silent "<b>bold</b>"

  # Test a bot: send a command and collect its replies until it goes quiet
  tg send --peer @mybot /start --wait 30s -o json

  # Keep a secret out of the process list
  printf '%s' "$CODE" | tg send --peer @mybot --stdin --wait 20s`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := messageText(args, fromStdin, cmd.InOrStdin())
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
				id, err := unpack.MessageID(b.StyledText(ctx, options...))
				if err != nil {
					return errors.Wrap(err, "send")
				}
				res := sentResult{Peer: peer, MessageID: id}
				if res.conversation, err = awaitResponse(ctx, collector, wait); err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&peer, "peer", "p", "",
		"peer to write (username, phone, deep link, or id:<n>); default: yourself")
	fs.BoolVar(&fromStdin, "stdin", false, "read the message text from standard input")
	msg.register(fs)
	wait.register(fs)
	registerPeerCompletion(cmd, "peer")

	return cmd
}
