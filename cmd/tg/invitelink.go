package main

import (
	"context"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

// noLinkResult is emitted by `invite-link show` when a chat has no primary
// invite link yet.
type noLinkResult struct {
	Link string `json:"link"`
}

// MarshalText explains that no link exists.
func (r noLinkResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintln(w, "no invite link; generate one with `tg invite-link new <peer>`")
	return err
}

// currentInviteLink returns the chat's primary (permanent) invite link, or the
// empty string if none has been generated yet. It works for both basic groups
// and channels/supergroups.
func currentInviteLink(ctx context.Context, api *tg.Client, p peers.Peer) (string, error) {
	var (
		full *tg.MessagesChatFull
		err  error
	)
	switch v := p.(type) {
	case peers.Channel:
		full, err = api.ChannelsGetFullChannel(ctx, v.InputChannel())
	case peers.Chat:
		full, err = api.MessagesGetFullChat(ctx, v.ID())
	default:
		return "", errors.New("peer is not a group or channel")
	}
	if err != nil {
		return "", errors.Wrap(err, "get full chat")
	}
	return inviteLinkFromFull(full)
}

// inviteLinkFromFull extracts the primary invite link from a full-chat
// response, returning the empty string when none has been generated yet.
func inviteLinkFromFull(full *tg.MessagesChatFull) (string, error) {
	inv, ok := full.FullChat.GetExportedInvite()
	if !ok {
		return "", nil
	}
	exp, ok := inv.(*tg.ChatInviteExported)
	if !ok {
		return "", errors.Errorf("unexpected invite type %T", inv)
	}
	return exp.Link, nil
}

func (a *app) newInviteLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite-link",
		Short:   "Manage a chat's invite link",
		GroupID: groupChats,
		Long:    "Show or generate the invite link of a group, supergroup or channel.",
	}
	cmd.AddCommand(a.newInviteLinkShowCmd(), a.newInviteLinkNewCmd())
	return cmd
}

func (a *app) newInviteLinkShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <peer>",
		Short: "Show the current invite link",
		Long: `Show the chat's current primary invite link without generating a new one.
If the chat has no invite link yet, use ` + "`tg invite-link new`" + ` to create one.`,
		Example:           "  tg invite-link show @group\n  tg invite-link show id:2201861038 --output json",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				p, err := resolvePeerArg(ctx, m, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}
				link, err := currentInviteLink(ctx, api, p)
				if err != nil {
					return err
				}
				if link == "" {
					return a.printer.Emit(noLinkResult{})
				}
				return a.printer.Emit(linkResult{Link: link})
			})
		},
	}
	return cmd
}

func (a *app) newInviteLinkNewCmd() *cobra.Command {
	var (
		title   string
		request bool
	)

	cmd := &cobra.Command{
		Use:     "new <peer>",
		Aliases: []string{"export", "generate"},
		Short:   "Generate a new invite link",
		Long: `Generate (export) a new invite link for a group, supergroup or channel.
Each call creates an additional link; the chat's primary link is unchanged.`,
		Example:           "  tg invite-link new @group\n  tg invite-link new @group --title \"From CLI\" --request",
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
				req := &tg.MessagesExportChatInviteRequest{
					Peer:          peer,
					RequestNeeded: request,
				}
				if title != "" {
					req.SetTitle(title)
				}
				res, err := api.MessagesExportChatInvite(ctx, req)
				if err != nil {
					return errors.Wrap(err, "messages.exportChatInvite")
				}
				inv, ok := res.(*tg.ChatInviteExported)
				if !ok {
					return errors.Errorf("unexpected invite type %T", res)
				}
				return a.printer.Emit(linkResult{Link: inv.Link})
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&title, "title", "", "admin-only description for the link")
	fs.BoolVar(&request, "request", false, "require admin approval for each join")

	return cmd
}
