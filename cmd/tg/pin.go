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

// pinResult is the result of a pin/unpin command.
type pinResult struct {
	OK bool `json:"ok"`
}

// MarshalText renders a short acknowledgement.
func (r pinResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintln(w, "ok")
	return err
}

// updatePin pins or unpins a message.
func updatePin(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, id int, unpin, silent, oneside bool) error {
	_, err := api.MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{
		Peer:      peer,
		ID:        id,
		Unpin:     unpin,
		Silent:    silent,
		PmOneside: oneside,
	})
	if err != nil {
		return errors.Wrap(err, "messages.updatePinnedMessage")
	}
	return nil
}

// pinCmd builds a pin or unpin command (unpin controls the verb).
func (a *app) pinCmd(use, short string, unpin bool) *cobra.Command {
	var (
		silent  bool
		oneside bool
	)

	cmd := &cobra.Command{
		Use:               use + " <peer> <message-id>",
		Short:             short,
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
				if err := updatePin(ctx, api, peer, id, unpin, silent, oneside); err != nil {
					return err
				}
				return a.printer.Emit(pinResult{OK: true})
			})
		},
	}

	if !unpin {
		fs := cmd.Flags()
		fs.BoolVar(&silent, "silent", false, "pin without notifying members")
		fs.BoolVar(&oneside, "oneside", false, "pin only on your side (private chats)")
	}

	return cmd
}

func (a *app) newPinCmd() *cobra.Command {
	return a.pinCmd("pin", "Pin a message", false)
}

func (a *app) newUnpinCmd() *cobra.Command {
	return a.pinCmd("unpin", "Unpin a message", true)
}

func (a *app) newUnpinAllCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:               "unpin-all <peer>",
		Short:             "Unpin all messages in a chat",
		GroupID:           groupMessaging,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to unpin all without --yes")
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
				if _, err := api.MessagesUnpinAllMessages(ctx, &tg.MessagesUnpinAllMessagesRequest{Peer: peer}); err != nil {
					return errors.Wrap(err, "messages.unpinAllMessages")
				}
				return a.printer.Emit(pinResult{OK: true})
			})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm unpinning all messages")

	return cmd
}

func (a *app) newPinnedCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:               "pinned <peer>",
		Short:             "List pinned messages",
		GroupID:           groupMessaging,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		Example:           "  tg pinned @durov\n  tg pinned me --output json",
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
				res, err := searchMessages(ctx, api, peer, "", &tg.InputMessagesFilterPinned{}, limit)
				if err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 50, "maximum number of pinned messages")

	return cmd
}
