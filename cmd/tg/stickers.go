package main

import (
	"context"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

// stickerSetItem describes an installed sticker set.
type stickerSetItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ShortName string `json:"short_name"`
	Count     int    `json:"count"`
}

// stickerSetsResult is the result of `tg stickers`.
type stickerSetsResult struct {
	Sets []stickerSetItem `json:"sets"`
}

// MarshalText renders one sticker set per line.
func (r stickerSetsResult) MarshalText(w io.Writer) error {
	for _, s := range r.Sets {
		if _, err := fmt.Fprintf(w, "%-32s %s (%d)\n", s.Title, s.ShortName, s.Count); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) newStickersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stickers",
		Short:   "List installed sticker sets",
		GroupID: groupMessaging,
		Args:    cobra.NoArgs,
		Example: "  tg stickers\n  tg stickers --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.MessagesGetAllStickers(ctx, 0)
				if err != nil {
					return errors.Wrap(err, "messages.getAllStickers")
				}
				all, ok := res.(*tg.MessagesAllStickers)
				if !ok {
					// MessagesAllStickersNotModified: nothing to show.
					return a.printer.Emit(stickerSetsResult{})
				}

				var out stickerSetsResult
				for _, s := range all.Sets {
					out.Sets = append(out.Sets, stickerSetItem{
						ID:        s.ID,
						Title:     s.Title,
						ShortName: s.ShortName,
						Count:     s.Count,
					})
				}
				return a.printer.Emit(out)
			})
		},
	}
	return cmd
}
