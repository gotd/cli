package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// linkResult is the result of `tg link`.
type linkResult struct {
	Link string `json:"link"`
	HTML string `json:"html,omitempty"`
}

// MarshalText prints the link.
func (r linkResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintln(w, r.Link)
	return err
}

func (a *app) newLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "link <peer> <message-id>",
		Short:   "Get a public link to a message",
		GroupID: groupMessaging,
		Long:    "Export a t.me link to a message. Only works for channels and supergroups.",
		Example: `  tg link @durov 12345
  tg link @somechannel 42 --output json`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return errors.Wrap(err, "message-id must be an integer")
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}
				ch, ok := peer.(*tg.InputPeerChannel)
				if !ok {
					return errors.New("message links are only available for channels and supergroups")
				}
				res, err := api.ChannelsExportMessageLink(ctx, &tg.ChannelsExportMessageLinkRequest{
					Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
					ID:      id,
				})
				if err != nil {
					return errors.Wrap(err, "channels.exportMessageLink")
				}
				return a.printer.Emit(linkResult{Link: res.Link, HTML: res.HTML})
			})
		},
	}
	return cmd
}
