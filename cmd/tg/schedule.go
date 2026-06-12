package main

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

func (a *app) newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schedule",
		Short:   "Manage scheduled messages",
		GroupID: groupMessaging,
		Long:    "Send, list, and delete scheduled messages.",
	}
	cmd.AddCommand(a.newScheduleSendCmd(), a.newScheduleListCmd(), a.newScheduleDeleteCmd())
	return cmd
}

func (a *app) newScheduleSendCmd() *cobra.Command {
	var (
		at string
		in time.Duration
	)

	cmd := &cobra.Command{
		Use:               "send <peer> <text>",
		Short:             "Schedule a message",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		Long:              "Schedule a message with --at (RFC3339 time) or --in (duration from now).",
		Example: `  tg schedule send @durov "happy new year" --at 2027-01-01T00:00:00Z
  tg schedule send me "reminder" --in 2h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			when, err := scheduleTime(at, in)
			if err != nil {
				return err
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				sender, err := a.sender(api)
				if err != nil {
					return err
				}
				id, err := unpack.MessageID(
					builderFor(sender, args[0]).Schedule(when).StyledText(ctx, styling.Plain(args[1])),
				)
				if err != nil {
					return errors.Wrap(err, "schedule send")
				}
				return a.printer.Emit(sentResult{Peer: args[0], MessageID: id})
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&at, "at", "", "absolute time (RFC3339), e.g. 2027-01-01T00:00:00Z")
	fs.DurationVar(&in, "in", 0, "relative delay from now, e.g. 2h")

	return cmd
}

// scheduleTime resolves the schedule time from --at or --in.
func scheduleTime(at string, in time.Duration) (time.Time, error) {
	switch {
	case at != "":
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "parse --at (want RFC3339)")
		}
		return t, nil
	case in > 0:
		return time.Now().Add(in), nil
	default:
		return time.Time{}, errors.New("specify --at <RFC3339> or --in <duration>")
	}
}

func (a *app) newScheduleListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list <peer>",
		Short:             "List scheduled messages",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}
				res, err := api.MessagesGetScheduledHistory(ctx, &tg.MessagesGetScheduledHistoryRequest{Peer: peer})
				if err != nil {
					return errors.Wrap(err, "messages.getScheduledHistory")
				}
				msgs, ent, err := messagesFrom(res)
				if err != nil {
					return err
				}
				return a.printer.Emit(messagesToHistory(msgs, ent))
			})
		},
	}
	return cmd
}

func (a *app) newScheduleDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete <peer> <message-id>...",
		Short:             "Delete scheduled messages",
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
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
				if _, err := api.MessagesDeleteScheduledMessages(ctx, &tg.MessagesDeleteScheduledMessagesRequest{
					Peer: peer,
					ID:   ids,
				}); err != nil {
					return errors.Wrap(err, "messages.deleteScheduledMessages")
				}
				return a.printer.Emit(deletedResult{Count: len(ids)})
			})
		},
	}
	return cmd
}
