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

// messageItem describes one message. Fields after ReplyTo were added later;
// all are omitted when empty, so older consumers see the same shape.
type messageItem struct {
	ID      int      `json:"id"`
	Date    int      `json:"date"`
	Out     bool     `json:"out"`
	From    *peerRef `json:"from,omitempty"`
	Text    string   `json:"text,omitempty"`
	Media   string   `json:"media,omitempty"`
	ReplyTo int      `json:"reply_to,omitempty"`

	// EditDate is the unix time of the last edit (0 = never edited).
	EditDate int `json:"edit_date,omitempty"`
	// ViaBotID is the inline bot the message was sent through.
	ViaBotID int64 `json:"via_bot_id,omitempty"`
	// GroupedID links the messages of one album.
	GroupedID int64 `json:"grouped_id,omitempty"`
	// File describes attached document or photo media.
	File *fileInfo `json:"file,omitempty"`
	// Entities are the text's formatting and links.
	Entities []entityItem `json:"entities,omitempty"`
	// ReplyMarkup is the keyboard a bot attached (inline buttons or a reply
	// keyboard); press buttons with `tg click`.
	ReplyMarkup *replyMarkup `json:"reply_markup,omitempty"`
}

// historyResult is the result of `tg history`.
type historyResult struct {
	Peer     peerRef       `json:"peer"`
	Messages []messageItem `json:"messages"`
}

// MarshalText renders messages oldest-first, one per line.
func (h historyResult) MarshalText(w io.Writer) error {
	for _, m := range h.Messages {
		dir := "<"
		if m.Out {
			dir = ">"
		}
		line := fmt.Sprintf("#%d %s", m.ID, dir)
		if m.From != nil {
			line += " " + m.From.label()
		}
		if m.Text != "" {
			line += " " + m.Text
		}
		if m.Media != "" {
			line += " [" + m.Media + "]"
		}
		if m.ReplyTo != 0 {
			line += fmt.Sprintf(" (reply to #%d)", m.ReplyTo)
		}
		if m.EditDate != 0 {
			line += " (edited)"
		}
		if m.ReplyMarkup != nil {
			line += " " + m.ReplyMarkup.label()
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// listHistory reads up to limit recent messages from peer (newest-first from the
// API), returning them oldest-first. With after > 0 only messages newer than
// that id are returned.
func listHistory(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, limit, after int) (historyResult, error) {
	// Fetch in large batches to minimize round trips. The iterator's default
	// batch size is 1, so it issues one messages.getHistory RPC per message,
	// which makes larger --limit values extremely slow. Telegram does not
	// reject a per-request limit above 100 but silently returns fewer/incorrect
	// results, so cap at 100. Clamp to at least 1: a non-positive batch size
	// would panic the iterator (it preallocates a slice with that capacity).
	batch := max(1, min(limit, 100))
	iter := query.Messages(api).GetHistory(peer).BatchSize(batch).Iter()

	var res historyResult
	for iter.Next(ctx) {
		if len(res.Messages) >= limit {
			break
		}
		elem := iter.Value()
		if after > 0 && elem.Msg.GetID() <= after {
			break // newest-first: everything from here on is older
		}
		msg, ok := elem.Msg.(*tg.Message)
		if !ok {
			continue
		}
		if res.Peer.Type == "" {
			res.Peer = describePeer(msg.PeerID, elem.Entities)
		}
		res.Messages = append(res.Messages, buildMessageItem(msg, elem.Entities))
	}
	if err := iter.Err(); err != nil {
		return historyResult{}, errors.Wrap(err, "iterate history")
	}

	// API returns newest-first; present chronologically.
	for i, j := 0, len(res.Messages)-1; i < j; i, j = i+1, j-1 {
		res.Messages[i], res.Messages[j] = res.Messages[j], res.Messages[i]
	}
	return res, nil
}

func (a *app) newHistoryCmd() *cobra.Command {
	var limit, after int

	cmd := &cobra.Command{
		Use:     "history <peer>",
		Aliases: []string{"messages"},
		Short:   "Read recent messages from a peer",
		GroupID: groupMessaging,
		Long: `Read recent messages from a peer (newest-first from the API, printed
oldest-first). The peer is me/self, @username, phone, or a t.me link.

JSON output includes each message's buttons ("reply_markup"), formatting
("entities"), attached file details ("file") and "edit_date". --after returns
only messages newer than a given id, e.g. the replies to a message you sent.`,
		Example: `  tg history me
  tg history @durov --limit 20
  tg history @somechannel --output json
  tg history @mybot --after 4242 -o json`,
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
				res, err := listHistory(ctx, api, peer, limit, after)
				if err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 30, "maximum number of messages to read")
	cmd.Flags().IntVar(&after, "after", 0, "only messages with an id greater than this")

	return cmd
}
