package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// reactionItem is one reaction with its count.
type reactionItem struct {
	Reaction string `json:"reaction"`
	Count    int    `json:"count"`
	Chosen   bool   `json:"chosen"`
}

// reactionsResult is the result of `tg reactions`.
type reactionsResult struct {
	Reactions []reactionItem `json:"reactions"`
}

// MarshalText renders reactions on one line.
func (r reactionsResult) MarshalText(w io.Writer) error {
	if len(r.Reactions) == 0 {
		_, err := fmt.Fprintln(w, "(no reactions)")
		return err
	}
	parts := make([]string, 0, len(r.Reactions))
	for _, it := range r.Reactions {
		s := fmt.Sprintf("%s %d", it.Reaction, it.Count)
		if it.Chosen {
			s += "*"
		}
		parts = append(parts, s)
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "  "))
	return err
}

// reactionLabel renders a reaction class as a short string.
func reactionLabel(r tg.ReactionClass) string {
	switch v := r.(type) {
	case *tg.ReactionEmoji:
		return v.Emoticon
	case *tg.ReactionCustomEmoji:
		return "custom:" + strconv.FormatInt(v.DocumentID, 10)
	default:
		return "?"
	}
}

// sendReaction sets or clears the reaction on a message.
func sendReaction(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, id int, emoji string) error {
	var reactions []tg.ReactionClass
	if emoji != "" {
		reactions = []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}}
	}
	if _, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     peer,
		MsgID:    id,
		Reaction: reactions,
	}); err != nil {
		return errors.Wrap(err, "messages.sendReaction")
	}
	return nil
}

func (a *app) reactCmd(use, short string, remove bool) *cobra.Command {
	minArgs := 3
	if remove {
		minArgs = 2
	}

	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		GroupID:           groupMessaging,
		Args:              cobra.ExactArgs(minArgs),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return errors.Wrap(err, "message-id must be an integer")
			}
			emoji := ""
			if !remove {
				emoji = args[2]
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
				if err := sendReaction(ctx, api, peer, id, emoji); err != nil {
					return err
				}
				return a.printer.Emit(pinResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newReactCmd() *cobra.Command {
	return a.reactCmd("react <peer> <message-id> <emoji>", "React to a message with an emoji", false)
}

func (a *app) newUnreactCmd() *cobra.Command {
	return a.reactCmd("unreact <peer> <message-id>", "Remove your reaction from a message", true)
}

func (a *app) newReactionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "reactions <peer> <message-id>",
		Short:             "Show reactions on a message",
		GroupID:           groupMessaging,
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
				msg, err := getMessage(ctx, api, peer, id)
				if err != nil {
					return err
				}

				var res reactionsResult
				if r, ok := msg.GetReactions(); ok {
					for _, rc := range r.Results {
						_, chosen := rc.GetChosenOrder()
						res.Reactions = append(res.Reactions, reactionItem{
							Reaction: reactionLabel(rc.Reaction),
							Count:    rc.Count,
							Chosen:   chosen,
						})
					}
				}
				return a.printer.Emit(res)
			})
		},
	}
	return cmd
}
