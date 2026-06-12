package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

// resolveUsers resolves peer strings to input users.
func resolveUsers(ctx context.Context, m *peers.Manager, args []string) ([]tg.InputUserClass, error) {
	users := make([]tg.InputUserClass, 0, len(args))
	for _, a := range args {
		p, err := m.Resolve(ctx, a)
		if err != nil {
			return nil, errors.Wrapf(err, "resolve %q", a)
		}
		u, ok := p.(peers.User)
		if !ok {
			return nil, errors.Errorf("%q is not a user", a)
		}
		users = append(users, u.InputUser())
	}
	return users, nil
}

// asInputChannel resolves a peer string to an input channel.
func asInputChannel(ctx context.Context, m *peers.Manager, arg string) (tg.InputChannelClass, error) {
	p, err := m.Resolve(ctx, arg)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve %q", arg)
	}
	ch, ok := p.(peers.Channel)
	if !ok {
		return nil, errors.Errorf("%q is not a channel or supergroup", arg)
	}
	return ch.InputChannel(), nil
}

func (a *app) newCreateGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create-group <title> <user>...",
		Short:   "Create a basic group with members",
		GroupID: groupChats,
		Args:    cobra.MinimumNArgs(2),
		Example: `  tg create-group "Project" @alice @bob`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				users, err := resolveUsers(ctx, m, args[1:])
				if err != nil {
					return err
				}
				res, err := api.MessagesCreateChat(ctx, &tg.MessagesCreateChatRequest{Title: args[0], Users: users})
				if err != nil {
					return errors.Wrap(err, "messages.createChat")
				}
				if ref, ok := firstChatRef(res.Updates); ok {
					return a.printer.Emit(ref)
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newCreateChannelCmd() *cobra.Command {
	var (
		about     string
		broadcast bool
		forum     bool
	)

	cmd := &cobra.Command{
		Use:     "create-channel <title>",
		Short:   "Create a channel or supergroup",
		GroupID: groupChats,
		Args:    cobra.ExactArgs(1),
		Long:    "Create a supergroup by default, or a broadcast channel with --broadcast.",
		Example: `  tg create-channel "News" --broadcast --about "Updates"
  tg create-channel "Team"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
					Title:     args[0],
					About:     about,
					Broadcast: broadcast,
					Megagroup: !broadcast,
					Forum:     forum,
				})
				if err != nil {
					return errors.Wrap(err, "channels.createChannel")
				}
				if ref, ok := firstChatRef(res); ok {
					return a.printer.Emit(ref)
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&about, "about", "", "channel description")
	fs.BoolVar(&broadcast, "broadcast", false, "create a broadcast channel (default: supergroup)")
	fs.BoolVar(&forum, "forum", false, "enable forum topics (supergroups)")

	return cmd
}

func (a *app) newInviteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "invite <peer> <user>...",
		Short:             "Invite users to a group or channel",
		GroupID:           groupChats,
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				users, err := resolveUsers(ctx, m, args[1:])
				if err != nil {
					return err
				}
				peerObj, err := m.Resolve(ctx, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}

				switch v := peerObj.(type) {
				case peers.Channel:
					_, err = api.ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{
						Channel: v.InputChannel(),
						Users:   users,
					})
				case peers.Chat:
					for _, u := range users {
						if _, err = api.MessagesAddChatUser(ctx, &tg.MessagesAddChatUserRequest{
							ChatID: v.ID(),
							UserID: u,
						}); err != nil {
							break
						}
					}
				default:
					return errors.New("peer is not a group or channel")
				}
				if err != nil {
					return errors.Wrap(err, "invite")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newLeaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "leave <peer>",
		Short:             "Leave a group or channel",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peerObj, err := m.Resolve(ctx, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}
				switch v := peerObj.(type) {
				case peers.Channel:
					_, err = api.ChannelsLeaveChannel(ctx, v.InputChannel())
				case peers.Chat:
					_, err = api.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
						ChatID: v.ID(),
						UserID: &tg.InputUserSelf{},
					})
				default:
					return errors.New("peer is not a group or channel")
				}
				if err != nil {
					return errors.Wrap(err, "leave")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}
