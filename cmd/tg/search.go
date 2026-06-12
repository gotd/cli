package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
)

// messagesToHistory maps raw messages + entities into a historyResult.
func messagesToHistory(msgs []tg.MessageClass, ent peer.Entities) historyResult {
	var out historyResult
	for _, mc := range msgs {
		msg, ok := mc.(*tg.Message)
		if !ok {
			continue
		}
		if out.Peer.Type == "" {
			out.Peer = describePeer(msg.PeerID, ent)
		}
		out.Messages = append(out.Messages, buildMessageItem(msg, ent))
	}
	return out
}

// searchMessages searches a single peer's messages (one page).
func searchMessages(
	ctx context.Context,
	api *tg.Client,
	p tg.InputPeerClass,
	q string,
	filter tg.MessagesFilterClass,
	limit int,
) (historyResult, error) {
	if filter == nil {
		filter = &tg.InputMessagesFilterEmpty{}
	}
	res, err := api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
		Peer:   p,
		Q:      q,
		Filter: filter,
		Limit:  limit,
	})
	if err != nil {
		return historyResult{}, errors.Wrap(err, "messages.search")
	}
	msgs, ent, err := messagesFrom(res)
	if err != nil {
		return historyResult{}, err
	}
	return messagesToHistory(msgs, ent), nil
}

// searchGlobal searches messages across all chats (one page).
func searchGlobal(ctx context.Context, api *tg.Client, q string, limit int) (historyResult, error) {
	res, err := api.MessagesSearchGlobal(ctx, &tg.MessagesSearchGlobalRequest{
		Q:          q,
		Filter:     &tg.InputMessagesFilterEmpty{},
		Limit:      limit,
		OffsetPeer: &tg.InputPeerEmpty{},
	})
	if err != nil {
		return historyResult{}, errors.Wrap(err, "messages.searchGlobal")
	}
	msgs, ent, err := messagesFrom(res)
	if err != nil {
		return historyResult{}, err
	}
	return messagesToHistory(msgs, ent), nil
}

func (a *app) newSearchCmd() *cobra.Command {
	var (
		global bool
		limit  int
	)

	cmd := &cobra.Command{
		Use:     "search <peer> <query>",
		Short:   "Search messages in a peer or globally",
		GroupID: groupMessaging,
		Long: `Search messages. With a peer, searches that chat's history; with --global
the peer is omitted and the query is run across all chats.`,
		Example: `  tg search @durov "release"
  tg search --global "invoice" --limit 20`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				if global {
					res, err := searchGlobal(ctx, api, args[0], limit)
					if err != nil {
						return err
					}
					return a.printer.Emit(res)
				}
				if len(args) != 2 {
					return errors.New("usage: tg search <peer> <query> (or --global <query>)")
				}
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}
				res, err := searchMessages(ctx, api, peer, args[1], nil, limit)
				if err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	fs := cmd.Flags()
	fs.BoolVarP(&global, "global", "g", false, "search across all chats (query only, no peer)")
	fs.IntVarP(&limit, "limit", "n", 50, "maximum number of results")

	return cmd
}
