package main

import (
	"context"
	"sort"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// messageContext fetches messages surrounding a message id (radius on each side).
func messageContext(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, id, radius int) (historyResult, error) {
	res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:      peer,
		OffsetID:  id,
		AddOffset: -radius,
		Limit:     2*radius + 1,
	})
	if err != nil {
		return historyResult{}, errors.Wrap(err, "messages.getHistory")
	}
	msgs, ent, err := messagesFrom(res)
	if err != nil {
		return historyResult{}, err
	}
	out := messagesToHistory(msgs, ent)
	sort.Slice(out.Messages, func(i, j int) bool {
		return out.Messages[i].ID < out.Messages[j].ID
	})
	return out, nil
}

func (a *app) newContextCmd() *cobra.Command {
	var radius int

	cmd := &cobra.Command{
		Use:     "context <peer> <message-id>",
		Short:   "Show messages around a message",
		GroupID: groupMessaging,
		Long:    "Show the messages immediately before and after a given message id.",
		Example: `  tg context @durov 12345
  tg context me 1000 --radius 10`,
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
				res, err := messageContext(ctx, api, peer, id, radius)
				if err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	cmd.Flags().IntVarP(&radius, "radius", "r", 5, "number of messages to show on each side")

	return cmd
}
