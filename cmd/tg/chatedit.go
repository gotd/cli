package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func (a *app) newSetTitleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set-title <peer> <title>",
		Short:             "Change a chat's title",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
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
				switch v := p.(type) {
				case peers.Channel:
					_, err = api.ChannelsEditTitle(ctx, &tg.ChannelsEditTitleRequest{Channel: v.InputChannel(), Title: args[1]})
				case peers.Chat:
					_, err = api.MessagesEditChatTitle(ctx, &tg.MessagesEditChatTitleRequest{ChatID: v.ID(), Title: args[1]})
				default:
					return errors.New("peer is not a group or channel")
				}
				if err != nil {
					return errors.Wrap(err, "edit title")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newSetAboutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set-about <peer> <about>",
		Short:             "Change a chat's description",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
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
				if _, err := api.MessagesEditChatAbout(ctx, &tg.MessagesEditChatAboutRequest{
					Peer:  peer,
					About: args[1],
				}); err != nil {
					return errors.Wrap(err, "messages.editChatAbout")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newSetPhotoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set-photo <peer> <file>",
		Short:             "Set a chat's photo",
		GroupID:           groupChats,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				file, err := uploader.NewUploader(api).FromPath(ctx, filepath.Clean(args[1]))
				if err != nil {
					return errors.Wrapf(err, "upload %q", args[1])
				}
				photo := &tg.InputChatUploadedPhoto{File: file}

				m, err := a.manager(api)
				if err != nil {
					return err
				}
				p, err := m.Resolve(ctx, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}
				switch v := p.(type) {
				case peers.Channel:
					_, err = api.ChannelsEditPhoto(ctx, &tg.ChannelsEditPhotoRequest{Channel: v.InputChannel(), Photo: photo})
				case peers.Chat:
					_, err = api.MessagesEditChatPhoto(ctx, &tg.MessagesEditChatPhotoRequest{ChatID: v.ID(), Photo: photo})
				default:
					return errors.New("peer is not a group or channel")
				}
				if err != nil {
					return errors.Wrap(err, "edit photo")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

// inviteHash extracts the invite hash from a t.me/+hash or joinchat link.
func inviteHash(link string) string {
	link = strings.TrimSpace(link)
	for _, p := range []string{"https://t.me/joinchat/", "https://t.me/+", "t.me/joinchat/", "t.me/+", "+"} {
		if strings.HasPrefix(link, p) {
			return strings.TrimPrefix(link, p)
		}
	}
	return link
}

func (a *app) newJoinLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "join-link <invite-link>",
		Short:   "Join a chat by invite link",
		GroupID: groupChats,
		Args:    cobra.ExactArgs(1),
		Example: "  tg join-link https://t.me/+AbCdEf123\n  tg join-link AbCdEf123",
		RunE: func(cmd *cobra.Command, args []string) error {
			hash := inviteHash(args[0])
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				if _, err := api.MessagesImportChatInvite(ctx, hash); err != nil {
					return errors.Wrap(err, "messages.importChatInvite")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}
