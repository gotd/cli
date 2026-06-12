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

// folderItem is one dialog filter (folder).
type folderItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Chats int    `json:"chats,omitempty"`
}

// foldersResult is the result of `tg folders list`.
type foldersResult struct {
	Folders []folderItem `json:"folders"`
}

// MarshalText renders one folder per line.
func (r foldersResult) MarshalText(w io.Writer) error {
	for _, f := range r.Folders {
		if _, err := fmt.Fprintf(w, "#%d %s\n", f.ID, f.Title); err != nil {
			return err
		}
	}
	return nil
}

// getFilters fetches all dialog filters.
func getFilters(ctx context.Context, api *tg.Client) ([]tg.DialogFilterClass, error) {
	res, err := api.MessagesGetDialogFilters(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "messages.getDialogFilters")
	}
	return res.Filters, nil
}

// findFilter returns the concrete *tg.DialogFilter with the given id.
func findFilter(filters []tg.DialogFilterClass, id int) (*tg.DialogFilter, error) {
	for _, f := range filters {
		if df, ok := f.(*tg.DialogFilter); ok && df.ID == id {
			return df, nil
		}
	}
	return nil, errors.Errorf("folder #%d not found", id)
}

func (a *app) newFoldersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "folders",
		Short:   "Manage chat folders (dialog filters)",
		GroupID: groupProfile,
		Long:    "List, create, modify, reorder and delete chat folders.",
	}
	cmd.AddCommand(
		a.newFoldersListCmd(),
		a.newFoldersCreateCmd(),
		a.newFoldersAddChatCmd(),
		a.newFoldersRemoveChatCmd(),
		a.newFoldersDeleteCmd(),
		a.newFoldersReorderCmd(),
	)
	return cmd
}

func (a *app) newFoldersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   cmdList,
		Short: "List chat folders",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				filters, err := getFilters(ctx, api)
				if err != nil {
					return err
				}
				var out foldersResult
				for _, f := range filters {
					df, ok := f.(*tg.DialogFilter)
					if !ok {
						continue
					}
					out.Folders = append(out.Folders, folderItem{
						ID:    df.ID,
						Title: df.Title.Text,
						Chats: len(df.IncludePeers),
					})
				}
				return a.printer.Emit(out)
			})
		},
	}
}

func (a *app) newFoldersCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <title>",
		Short: "Create a chat folder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				filters, err := getFilters(ctx, api)
				if err != nil {
					return err
				}
				id := 2
				for _, f := range filters {
					if df, ok := f.(interface{ GetID() int }); ok && df.GetID() >= id {
						id = df.GetID() + 1
					}
				}
				filter := &tg.DialogFilter{ID: id, Title: tg.TextWithEntities{Text: args[0]}}
				req := &tg.MessagesUpdateDialogFilterRequest{ID: id}
				req.SetFilter(filter)
				if _, err := api.MessagesUpdateDialogFilter(ctx, req); err != nil {
					return errors.Wrap(err, "messages.updateDialogFilter")
				}
				return a.printer.Emit(folderItem{ID: id, Title: args[0]})
			})
		},
	}
}

// modifyFolderChat adds or removes a peer from a folder's include list.
func (a *app) modifyFolderChat(use, short string, add bool) *cobra.Command {
	return &cobra.Command{
		Use:               use + " <folder-id> <peer>",
		Short:             short,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: noFileComp,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.Wrap(err, "folder-id must be an integer")
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[1])
				if err != nil {
					return err
				}
				filters, err := getFilters(ctx, api)
				if err != nil {
					return err
				}
				df, err := findFilter(filters, id)
				if err != nil {
					return err
				}
				df.IncludePeers = updatePeerList(df.IncludePeers, peer, add)

				req := &tg.MessagesUpdateDialogFilterRequest{ID: id}
				req.SetFilter(df)
				if _, err := api.MessagesUpdateDialogFilter(ctx, req); err != nil {
					return errors.Wrap(err, "messages.updateDialogFilter")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

// updatePeerList adds or removes a peer from a list (by peer id).
func updatePeerList(list []tg.InputPeerClass, peer tg.InputPeerClass, add bool) []tg.InputPeerClass {
	id := inputPeerID(peer)
	out := list[:0:0]
	for _, p := range list {
		if inputPeerID(p) == id {
			continue // drop existing entry (re-added below if needed)
		}
		out = append(out, p)
	}
	if add {
		out = append(out, peer)
	}
	return out
}

// inputPeerID returns a stable identifier for an input peer.
func inputPeerID(p tg.InputPeerClass) int64 {
	switch v := p.(type) {
	case *tg.InputPeerUser:
		return v.UserID
	case *tg.InputPeerChat:
		return v.ChatID
	case *tg.InputPeerChannel:
		return v.ChannelID
	case *tg.InputPeerSelf:
		return -1
	default:
		return 0
	}
}

func (a *app) newFoldersAddChatCmd() *cobra.Command {
	return a.modifyFolderChat("add-chat", "Add a chat to a folder", true)
}

func (a *app) newFoldersRemoveChatCmd() *cobra.Command {
	return a.modifyFolderChat("remove-chat", "Remove a chat from a folder", false)
}

func (a *app) newFoldersDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <folder-id>",
		Short: "Delete a chat folder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.Wrap(err, "folder-id must be an integer")
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				// Omitting Filter deletes the folder.
				if _, err := api.MessagesUpdateDialogFilter(ctx, &tg.MessagesUpdateDialogFilterRequest{ID: id}); err != nil {
					return errors.Wrap(err, "messages.updateDialogFilter")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

func (a *app) newFoldersReorderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reorder <folder-id>...",
		Short: "Reorder chat folders",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			order, err := parseIDs(args)
			if err != nil {
				return err
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				if _, err := api.MessagesUpdateDialogFiltersOrder(ctx, order); err != nil {
					return errors.Wrap(err, "messages.updateDialogFiltersOrder")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}
