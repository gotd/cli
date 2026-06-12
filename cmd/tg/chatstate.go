package main

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// okResult is a generic ok acknowledgement.
type okResult struct {
	OK bool `json:"ok"`
}

// MarshalText prints "ok".
func (r okResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintln(w, "ok")
	return err
}

// setMute mutes or unmutes a peer.
func setMute(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, mute bool) error {
	muteUntil := 0
	if mute {
		muteUntil = math.MaxInt32 // effectively forever
	}
	settings := tg.InputPeerNotifySettings{}
	settings.SetMuteUntil(muteUntil)

	if _, err := api.AccountUpdateNotifySettings(ctx, &tg.AccountUpdateNotifySettingsRequest{
		Peer:     &tg.InputNotifyPeer{Peer: peer},
		Settings: settings,
	}); err != nil {
		return errors.Wrap(err, "account.updateNotifySettings")
	}
	return nil
}

// setFolder moves a peer to a folder (0 = main, 1 = archive).
func setFolder(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, folder int) error {
	if _, err := api.FoldersEditPeerFolders(ctx, []tg.InputFolderPeer{{Peer: peer, FolderID: folder}}); err != nil {
		return errors.Wrap(err, "folders.editPeerFolders")
	}
	return nil
}

// peerAction runs against a resolved peer.
type peerAction func(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) error

// peerActionCmd builds a command that resolves a peer and runs an action on it.
func (a *app) peerActionCmd(use, short string, action peerAction) *cobra.Command {
	return &cobra.Command{
		Use:               use + " <peer>",
		Short:             short,
		GroupID:           groupChats,
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
				if err := action(ctx, api, peer); err != nil {
					return err
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

func (a *app) newMuteCmd() *cobra.Command {
	mute := func(ctx context.Context, api *tg.Client, p tg.InputPeerClass) error {
		return setMute(ctx, api, p, true)
	}
	return a.peerActionCmd("mute", "Mute a chat", mute)
}

func (a *app) newUnmuteCmd() *cobra.Command {
	unmute := func(ctx context.Context, api *tg.Client, p tg.InputPeerClass) error {
		return setMute(ctx, api, p, false)
	}
	return a.peerActionCmd("unmute", "Unmute a chat", unmute)
}

func (a *app) newArchiveCmd() *cobra.Command {
	archive := func(ctx context.Context, api *tg.Client, p tg.InputPeerClass) error {
		return setFolder(ctx, api, p, 1)
	}
	return a.peerActionCmd("archive", "Archive a chat", archive)
}

func (a *app) newUnarchiveCmd() *cobra.Command {
	unarchive := func(ctx context.Context, api *tg.Client, p tg.InputPeerClass) error {
		return setFolder(ctx, api, p, 0)
	}
	return a.peerActionCmd("unarchive", "Move a chat out of the archive", unarchive)
}
