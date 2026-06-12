package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

// defaultAdminRights is a sensible admin rights set for promotion.
func defaultAdminRights() tg.ChatAdminRights {
	return tg.ChatAdminRights{
		ChangeInfo:     true,
		DeleteMessages: true,
		BanUsers:       true,
		InviteUsers:    true,
		PinMessages:    true,
		ManageCall:     true,
	}
}

// editAdmin promotes (rights set) or demotes (empty rights) a user.
func editAdmin(
	ctx context.Context,
	api *tg.Client,
	ch tg.InputChannelClass,
	user tg.InputUserClass,
	rights tg.ChatAdminRights,
	rank string,
) error {
	if _, err := api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
		Channel:     ch,
		UserID:      user,
		AdminRights: rights,
		Rank:        rank,
	}); err != nil {
		return errors.Wrap(err, "channels.editAdmin")
	}
	return nil
}

// editBanned bans (ViewMessages) or unbans (empty) a participant.
func editBanned(ctx context.Context, api *tg.Client, ch tg.InputChannelClass, participant tg.InputPeerClass, ban bool) error {
	rights := tg.ChatBannedRights{}
	if ban {
		rights.ViewMessages = true
		rights.SendMessages = true
		rights.SendMedia = true
	}
	if _, err := api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel:      ch,
		Participant:  participant,
		BannedRights: rights,
	}); err != nil {
		return errors.Wrap(err, "channels.editBanned")
	}
	return nil
}

// channelAndUser resolves a channel peer and a user peer.
func (a *app) channelAndUser(ctx context.Context, api *tg.Client, chArg, userArg string) (tg.InputChannelClass, tg.InputUserClass, error) {
	m, err := a.manager(api)
	if err != nil {
		return nil, nil, err
	}
	ch, err := asInputChannel(ctx, m, chArg)
	if err != nil {
		return nil, nil, err
	}
	users, err := resolveUsers(ctx, m, []string{userArg})
	if err != nil {
		return nil, nil, err
	}
	return ch, users[0], nil
}

func (a *app) newPromoteCmd() *cobra.Command {
	var rank string
	cmd := &cobra.Command{
		Use:               "promote <peer> <user>",
		Short:             "Promote a user to admin",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				ch, user, err := a.channelAndUser(ctx, api, args[0], args[1])
				if err != nil {
					return err
				}
				if err := editAdmin(ctx, api, ch, user, defaultAdminRights(), rank); err != nil {
					return err
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	cmd.Flags().StringVar(&rank, "rank", "", "custom admin rank/title")
	return cmd
}

func (a *app) newDemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "demote <peer> <user>",
		Short:             "Remove a user's admin rights",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				ch, user, err := a.channelAndUser(ctx, api, args[0], args[1])
				if err != nil {
					return err
				}
				if err := editAdmin(ctx, api, ch, user, tg.ChatAdminRights{}, ""); err != nil {
					return err
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

// banUnbanCmd builds a ban or unban command.
func (a *app) banUnbanCmd(use, short string, ban bool) *cobra.Command {
	return &cobra.Command{
		Use:               use + " <peer> <user>",
		Short:             short,
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				ch, err := asInputChannel(ctx, m, args[0])
				if err != nil {
					return err
				}
				p, err := m.Resolve(ctx, args[1])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[1])
				}
				participant := p.InputPeer()
				if u, ok := p.(peers.User); ok {
					participant = u.InputPeer()
				}
				if err := editBanned(ctx, api, ch, participant, ban); err != nil {
					return err
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

func (a *app) newBanCmd() *cobra.Command {
	return a.banUnbanCmd("ban", "Ban a user from a group or channel", true)
}

func (a *app) newUnbanCmd() *cobra.Command {
	return a.banUnbanCmd("unban", "Unban a user", false)
}

func (a *app) newSlowModeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "slow-mode <peer> <seconds>",
		Short:             "Set slow mode (0 to disable)",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		Example:           "  tg slow-mode @group 30\n  tg slow-mode @group 0",
		RunE: func(cmd *cobra.Command, args []string) error {
			seconds, err := parseIDs([]string{args[1]})
			if err != nil {
				return errors.Wrap(err, "seconds must be an integer")
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				ch, err := asInputChannel(ctx, m, args[0])
				if err != nil {
					return err
				}
				if _, err := api.ChannelsToggleSlowMode(ctx, &tg.ChannelsToggleSlowModeRequest{
					Channel: ch,
					Seconds: seconds[0],
				}); err != nil {
					return errors.Wrap(err, "channels.toggleSlowMode")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}
