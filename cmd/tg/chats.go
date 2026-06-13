package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
)

// chatItem describes one dialog.
type chatItem struct {
	Peer        peerRef `json:"peer"`
	Unread      int     `json:"unread"`
	Pinned      bool    `json:"pinned"`
	Muted       bool    `json:"muted"`
	Archived    bool    `json:"archived"`
	LastMessage string  `json:"last_message,omitempty"`
	LastDate    int     `json:"last_date,omitempty"`
}

// chatList is the result of `tg chats list`.
type chatList struct {
	Chats []chatItem `json:"chats"`
}

// MarshalText renders one chat per line.
func (l chatList) MarshalText(w io.Writer) error {
	for _, c := range l.Chats {
		var flags []string
		if c.Pinned {
			flags = append(flags, "pinned")
		}
		if c.Muted {
			flags = append(flags, "muted")
		}
		if c.Archived {
			flags = append(flags, "archived")
		}
		line := fmt.Sprintf("%-24s unread=%d", c.Peer.label(), c.Unread)
		if len(flags) > 0 {
			line += " [" + strings.Join(flags, ",") + "]"
		}
		if c.LastMessage != "" {
			line += "  " + truncate(c.LastMessage, 60)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// listChats fetches up to limit dialogs (archived folder when archived is set).
// When m is non-nil, the dialogs' peer entities are persisted to the access-hash
// cache, so peers without a username/phone can later be addressed by "id:<n>".
func listChats(ctx context.Context, api *tg.Client, m *peerManager, limit int, archived bool) (chatList, error) {
	folder := 0
	if archived {
		folder = 1
	}

	iter := query.GetDialogs(api).FolderID(folder).Iter()
	now := time.Now().Unix()

	var (
		out   chatList
		users = map[int64]*tg.User{}
		chats = map[int64]tg.ChatClass{}
	)
	for iter.Next(ctx) {
		if len(out.Chats) >= limit {
			break
		}
		elem := iter.Value()
		dlg, ok := elem.Dialog.(*tg.Dialog)
		if !ok {
			continue
		}

		item := chatItem{
			Peer:     describePeer(dlg.Peer, elem.Entities),
			Unread:   dlg.UnreadCount,
			Pinned:   dlg.Pinned,
			Archived: dlg.FolderID == 1,
		}
		if mute, ok := dlg.NotifySettings.GetMuteUntil(); ok && int64(mute) > now {
			item.Muted = true
		}
		if msg, ok := elem.Last.(*tg.Message); ok {
			item.LastMessage = messagePreview(msg)
			item.LastDate = msg.Date
		}
		out.Chats = append(out.Chats, item)

		for id, u := range elem.Entities.Users() {
			users[id] = u
		}
		for id, c := range elem.Entities.Chats() {
			chats[id] = c
		}
		for id, c := range elem.Entities.Channels() {
			chats[id] = c
		}
	}
	if err := iter.Err(); err != nil {
		return chatList{}, errors.Wrap(err, "iterate dialogs")
	}

	if m != nil {
		us := make([]tg.UserClass, 0, len(users))
		for _, u := range users {
			us = append(us, u)
		}
		cs := make([]tg.ChatClass, 0, len(chats))
		for _, c := range chats {
			cs = append(cs, c)
		}
		if err := m.Apply(ctx, us, cs); err != nil {
			return chatList{}, errors.Wrap(err, "cache peers")
		}
	}
	return out, nil
}

func (a *app) newChatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "chats",
		Short:   "List and inspect chats",
		GroupID: groupMessaging,
		Long:    "Commands for working with the dialog (chat) list.",
	}
	cmd.AddCommand(a.newChatsListCmd())
	return cmd
}

func (a *app) newChatsListCmd() *cobra.Command {
	var (
		limit    int
		archived bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List dialogs with unread counts and flags",
		Long: `List dialogs (chats) newest-first, with unread counts, pinned/muted/archived
flags and a last-message preview.`,
		Example: `  tg chats list
  tg chats list --limit 50 --output json
  tg chats list --archived`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				list, err := listChats(ctx, api, m, limit, archived)
				if err != nil {
					return err
				}
				return a.printer.Emit(list)
			})
		},
	}

	fs := cmd.Flags()
	fs.IntVarP(&limit, "limit", "n", 100, "maximum number of chats to list")
	fs.BoolVar(&archived, "archived", false, "list archived chats instead of the main list")

	return cmd
}
