package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// participantsFilter maps a filter name to a ChannelParticipantsFilterClass.
func participantsFilter(name string) (tg.ChannelParticipantsFilterClass, error) {
	switch name {
	case "recent", "":
		return &tg.ChannelParticipantsRecent{}, nil
	case "admins":
		return &tg.ChannelParticipantsAdmins{}, nil
	case "banned":
		return &tg.ChannelParticipantsBanned{}, nil
	case "kicked":
		return &tg.ChannelParticipantsKicked{}, nil
	case "bots":
		return &tg.ChannelParticipantsBots{}, nil
	default:
		return nil, errors.Errorf("unknown filter %q", name)
	}
}

func participantFilters() []string {
	return []string{"recent", "admins", "banned", "kicked", "bots"}
}

// listParticipants fetches participants of a channel/supergroup with a filter.
func listParticipants(
	ctx context.Context,
	api *tg.Client,
	channel tg.InputChannelClass,
	filterName string,
	limit int,
) (peerListResult, error) {
	filter, err := participantsFilter(filterName)
	if err != nil {
		return peerListResult{}, err
	}
	res, err := api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: channel,
		Filter:  filter,
		Limit:   limit,
	})
	if err != nil {
		return peerListResult{}, errors.Wrap(err, "channels.getParticipants")
	}
	full, ok := res.(*tg.ChannelsChannelParticipants)
	if !ok {
		return peerListResult{}, errors.Errorf("unexpected participants type %T", res)
	}
	return usersToPeerList(full.Users), nil
}

func (a *app) participantsCmd(use, short, filter string) *cobra.Command {
	var (
		limit      int
		filterFlag string
	)

	cmd := &cobra.Command{
		Use:               use + " <peer>",
		Short:             short,
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := filter
			if f == "" {
				f = filterFlag
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				channel, err := asInputChannel(ctx, m, args[0])
				if err != nil {
					return err
				}
				res, err := listParticipants(ctx, api, channel, f, limit)
				if err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "maximum number of participants")
	if filter == "" {
		cmd.Flags().StringVar(&filterFlag, "filter", "recent", "filter: recent, admins, banned, kicked, bots")
		registerEnumCompletion(cmd, "filter", participantFilters())
	}

	return cmd
}

func (a *app) newParticipantsCmd() *cobra.Command {
	return a.participantsCmd("participants", "List members of a group or channel", "")
}

func (a *app) newAdminsCmd() *cobra.Command {
	return a.participantsCmd("admins", "List admins of a group or channel", "admins")
}

func (a *app) newBannedCmd() *cobra.Command {
	return a.participantsCmd("banned", "List banned users of a group or channel", "banned")
}
