package main

import (
	"context"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

func (a *app) newEditCmd() *cobra.Command {
	var useHTML bool

	cmd := &cobra.Command{
		Use:     "edit <peer> <message-id> <text>",
		Short:   "Edit a message",
		GroupID: groupMessaging,
		Long:    "Edit the text of a message you sent. The peer is me/self, @username, phone, or a t.me link.",
		Example: `  tg edit @durov 12345 "updated text"
  tg edit me 1000 --html "<b>bold</b>"`,
		Args:              cobra.ExactArgs(3),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return errors.Wrap(err, "message-id must be an integer")
			}
			text := args[2]

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				sender, m, err := a.sender(api)
				if err != nil {
					return err
				}
				bf, err := builderFor(ctx, m, sender, args[0])
				if err != nil {
					return err
				}

				opt := styling.Plain(text)
				if useHTML {
					opt = html.String(nil, text)
				}
				newID, err := unpack.MessageID(bf.Edit(id).StyledText(ctx, opt))
				if err != nil {
					return errors.Wrap(err, "edit")
				}
				return a.printer.Emit(sentResult{Peer: args[0], MessageID: newID})
			})
		},
	}

	cmd.Flags().BoolVar(&useHTML, "html", false, "use HTML styling")

	return cmd
}
