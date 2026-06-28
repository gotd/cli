package main

import (
	"context"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
)

// draftItem is one saved draft.
type draftItem struct {
	Peer    peerRef `json:"peer"`
	Message string  `json:"message"`
	Date    int     `json:"date"`
}

// draftsResult is the result of `tg drafts`.
type draftsResult struct {
	Drafts []draftItem `json:"drafts"`
}

// MarshalText renders one draft per line.
func (r draftsResult) MarshalText(w io.Writer) error {
	for _, d := range r.Drafts {
		if _, err := fmt.Fprintf(w, "%-24s %s\n", d.Peer.label(), truncate(d.Message, 60)); err != nil {
			return err
		}
	}
	return nil
}

// saveDraft sets or clears the draft for a peer (empty message clears it).
func saveDraft(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, text string) error {
	if _, err := api.MessagesSaveDraft(ctx, &tg.MessagesSaveDraftRequest{
		Peer:    peer,
		Message: text,
	}); err != nil {
		return errors.Wrap(err, "messages.saveDraft")
	}
	return nil
}

// listDrafts collects drafts from the dialog list.
func listDrafts(ctx context.Context, api *tg.Client) (draftsResult, error) {
	// Scan every dialog to collect drafts. The iterator's default batch size is
	// 1 (one messages.getDialogs RPC per dialog), so without batching this walks
	// the whole account one round trip at a time. 100 is the per-request maximum
	// Telegram serves.
	iter := query.GetDialogs(api).BatchSize(100).Iter()
	var out draftsResult
	for iter.Next(ctx) {
		elem := iter.Value()
		dlg, ok := elem.Dialog.(*tg.Dialog)
		if !ok {
			continue
		}
		draft, ok := dlg.Draft.(*tg.DraftMessage)
		if !ok || draft.Message == "" {
			continue
		}
		out.Drafts = append(out.Drafts, draftItem{
			Peer:    describePeer(dlg.Peer, elem.Entities),
			Message: draft.Message,
			Date:    draft.Date,
		})
	}
	if err := iter.Err(); err != nil {
		return draftsResult{}, errors.Wrap(err, "iterate dialogs")
	}
	return out, nil
}

func (a *app) newDraftCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "draft",
		Short:   "Manage message drafts",
		GroupID: groupMessaging,
		Long:    "Set, list, or clear message drafts.",
	}
	cmd.AddCommand(a.newDraftSetCmd(), a.newDraftClearCmd())
	return cmd
}

func (a *app) newDraftSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "set <peer> <text>",
		Short:             "Set the draft for a peer",
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
				if err := saveDraft(ctx, api, peer, args[1]); err != nil {
					return err
				}
				return a.printer.Emit(pinResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newDraftClearCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "clear <peer>",
		Short:             "Clear the draft for a peer",
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
				if err := saveDraft(ctx, api, peer, ""); err != nil {
					return err
				}
				return a.printer.Emit(pinResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newDraftsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "drafts",
		Short:   "List message drafts",
		GroupID: groupMessaging,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := listDrafts(ctx, api)
				if err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}
	return cmd
}
