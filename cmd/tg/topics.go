package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// randomID returns a random int64 suitable for MTProto random_id fields.
func randomID() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, errors.Wrap(err, "read random")
	}
	return int64(binary.LittleEndian.Uint64(b[:])), nil //nolint:gosec // random_id, not security-sensitive
}

// topicItem is one forum topic.
type topicItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// topicsResult is the result of `tg topics list`.
type topicsResult struct {
	Topics []topicItem `json:"topics"`
}

// MarshalText renders one topic per line.
func (r topicsResult) MarshalText(w io.Writer) error {
	for _, t := range r.Topics {
		if _, err := fmt.Fprintf(w, "#%d %s\n", t.ID, t.Title); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) newTopicsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "topics",
		Short:   "Manage forum topics",
		GroupID: groupChats,
		Long:    "List, create, and enable forum topics in supergroups.",
	}
	cmd.AddCommand(a.newTopicsListCmd(), a.newTopicsCreateCmd(), a.newTopicsEnableCmd())
	return cmd
}

func (a *app) newTopicsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "list <peer>",
		Short:             "List forum topics",
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
				res, err := api.MessagesGetForumTopics(ctx, &tg.MessagesGetForumTopicsRequest{Peer: peer})
				if err != nil {
					return errors.Wrap(err, "messages.getForumTopics")
				}
				var out topicsResult
				for _, tc := range res.Topics {
					if t, ok := tc.(*tg.ForumTopic); ok {
						out.Topics = append(out.Topics, topicItem{ID: t.ID, Title: t.Title})
					}
				}
				return a.printer.Emit(out)
			})
		},
	}
}

func (a *app) newTopicsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "create <peer> <title>",
		Short:             "Create a forum topic",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			rndID, err := randomID()
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
				if _, err := api.MessagesCreateForumTopic(ctx, &tg.MessagesCreateForumTopicRequest{
					Peer:     peer,
					Title:    args[1],
					RandomID: rndID,
				}); err != nil {
					return errors.Wrap(err, "messages.createForumTopic")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

func (a *app) newTopicsEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "enable <peer>",
		Short:             "Enable forum topics in a supergroup",
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
				if _, err := api.ChannelsToggleForum(ctx, &tg.ChannelsToggleForumRequest{
					Channel: ch,
					Enabled: true,
				}); err != nil {
					return errors.Wrap(err, "channels.toggleForum")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}
