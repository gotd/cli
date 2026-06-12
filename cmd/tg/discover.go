package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

func (a *app) newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resolve <username>",
		Short:   "Resolve a username to a peer",
		GroupID: groupChats,
		Long:    "Resolve a @username (or t.me link) to its peer id, type and name.",
		Example: "  tg resolve @durov\n  tg resolve durov --output json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				p, err := m.Resolve(ctx, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}
				return a.printer.Emit(describeManagedPeer(p))
			})
		},
	}
	return cmd
}

func (a *app) newSearchPublicCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:     "search-public <query>",
		Short:   "Search public chats and users",
		GroupID: groupChats,
		Long:    "Search Telegram's public directory of users, groups and channels.",
		Example: "  tg search-public gopher\n  tg search-public news --limit 20",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: args[0], Limit: limit})
				if err != nil {
					return errors.Wrap(err, "contacts.search")
				}
				ent := entitiesOf(found.Users, found.Chats)

				var out peerListResult
				seen := map[int64]bool{}
				for _, group := range [][]tg.PeerClass{found.MyResults, found.Results} {
					for _, pc := range group {
						ref := describePeer(pc, ent)
						if seen[ref.ID] {
							continue
						}
						seen[ref.ID] = true
						out.Peers = append(out.Peers, ref)
					}
				}
				return a.printer.Emit(out)
			})
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "maximum number of results")

	return cmd
}

func (a *app) newSubscribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "subscribe <peer>",
		Aliases: []string{"join"},
		Short:   "Join a public channel or group",
		GroupID: groupChats,
		Long:    "Join (subscribe to) a public channel or supergroup by username or link.",
		Example: "  tg subscribe @durov\n  tg join @somechannel",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				p, err := m.Resolve(ctx, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}
				ch, ok := p.(peers.Channel)
				if !ok {
					return errors.New("not a channel or supergroup")
				}
				if _, err := api.ChannelsJoinChannel(ctx, ch.InputChannel()); err != nil {
					return errors.Wrap(err, "channels.joinChannel")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}
