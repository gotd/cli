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

// deletedResult is the result of a delete command.
type deletedResult struct {
	Count int `json:"count"`
}

// MarshalText renders a short summary.
func (r deletedResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "deleted %d\n", r.Count)
	return err
}

// parseIDs converts string args to message ids.
func parseIDs(args []string) ([]int, error) {
	ids := make([]int, 0, len(args))
	for _, a := range args {
		id, err := strconv.Atoi(a)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid message id %q", a)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// deleteMessages deletes messages by id, branching on peer type.
func deleteMessages(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, ids []int, revoke bool) error {
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		if _, err := api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			ID:      ids,
		}); err != nil {
			return errors.Wrap(err, "channels.deleteMessages")
		}
		return nil
	}
	if _, err := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		Revoke: revoke,
		ID:     ids,
	}); err != nil {
		return errors.Wrap(err, "messages.deleteMessages")
	}
	return nil
}

func (a *app) newDeleteCmd() *cobra.Command {
	var (
		yes    bool
		revoke bool
	)

	cmd := &cobra.Command{
		Use:     "delete <peer> <message-id>...",
		Aliases: []string{"del", "rm"},
		Short:   "Delete one or more messages",
		GroupID: groupMessaging,
		Long: `Delete one or more messages by id. Destructive: requires --yes.
By default deletes for everyone (revoke); pass --revoke=false to delete only
your own copy.`,
		Example: `  tg delete @durov 12345 --yes
  tg delete me 1 2 3 --yes`,
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to delete without --yes")
			}
			ids, err := parseIDs(args[1:])
			if err != nil {
				return err
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
				if err := deleteMessages(ctx, api, peer, ids, revoke); err != nil {
					return err
				}
				return a.printer.Emit(deletedResult{Count: len(ids)})
			})
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&yes, "yes", false, "confirm deletion")
	fs.BoolVar(&revoke, "revoke", true, "delete for everyone (not just your copy)")

	return cmd
}

// deleteHistory clears a peer's history.
func deleteHistory(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, revoke, justClear bool) error {
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		if _, err := api.ChannelsDeleteHistory(ctx, &tg.ChannelsDeleteHistoryRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			MaxID:   0,
		}); err != nil {
			return errors.Wrap(err, "channels.deleteHistory")
		}
		return nil
	}
	if _, err := api.MessagesDeleteHistory(ctx, &tg.MessagesDeleteHistoryRequest{
		JustClear: justClear,
		Revoke:    revoke,
		Peer:      peer,
		MaxID:     0,
	}); err != nil {
		return errors.Wrap(err, "messages.deleteHistory")
	}
	return nil
}

func (a *app) newDeleteHistoryCmd() *cobra.Command {
	var (
		yes    bool
		revoke bool
	)

	cmd := &cobra.Command{
		Use:     "delete-history <peer>",
		Short:   "Delete a chat's history",
		GroupID: groupMessaging,
		Long: `Delete the message history of a peer. Destructive: requires --yes.
By default only clears your own copy; pass --revoke to delete for everyone.`,
		Example: `  tg delete-history @durov --yes
  tg delete-history me --yes --revoke`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to delete history without --yes")
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
				if err := deleteHistory(ctx, api, peer, revoke, !revoke); err != nil {
					return err
				}
				return a.printer.Emit(deletedResult{Count: 1})
			})
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&yes, "yes", false, "confirm deletion")
	fs.BoolVar(&revoke, "revoke", false, "delete for everyone (not just your copy)")

	return cmd
}
