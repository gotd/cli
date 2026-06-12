package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// adminEvent is one admin-log entry.
type adminEvent struct {
	ID     int64  `json:"id"`
	Date   int    `json:"date"`
	UserID int64  `json:"user_id"`
	Action string `json:"action"`
}

// recentActionsResult is the result of `tg recent-actions`.
type recentActionsResult struct {
	Events []adminEvent `json:"events"`
}

// MarshalText renders one event per line.
func (r recentActionsResult) MarshalText(w io.Writer) error {
	for _, e := range r.Events {
		if _, err := fmt.Fprintf(w, "#%d user=%d %s\n", e.ID, e.UserID, e.Action); err != nil {
			return err
		}
	}
	return nil
}

// actionName returns a short name for an admin-log action type.
func actionName(action tg.ChannelAdminLogEventActionClass) string {
	return strings.TrimPrefix(action.TypeName(), "channelAdminLogEventAction")
}

func (a *app) newRecentActionsCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:               "recent-actions <peer>",
		Short:             "Show the admin action log",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(1),
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
				res, err := api.ChannelsGetAdminLog(ctx, &tg.ChannelsGetAdminLogRequest{
					Channel: ch,
					Limit:   limit,
				})
				if err != nil {
					return errors.Wrap(err, "channels.getAdminLog")
				}

				var out recentActionsResult
				for _, e := range res.Events {
					out.Events = append(out.Events, adminEvent{
						ID:     e.ID,
						Date:   e.Date,
						UserID: e.UserID,
						Action: actionName(e.Action),
					})
				}
				return a.printer.Emit(out)
			})
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "maximum number of events")

	return cmd
}
