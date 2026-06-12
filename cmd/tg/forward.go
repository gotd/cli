package main

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"
)

func (a *app) newForwardCmd() *cobra.Command {
	var (
		from       string
		dropAuthor bool
	)

	cmd := &cobra.Command{
		Use:     "forward <to-peer> <message-id>...",
		Aliases: []string{"fwd"},
		Short:   "Forward messages to a peer",
		GroupID: groupMessaging,
		Long: `Forward one or more messages from a source chat (--from) to a target peer.
Peers are me/self, @username, phone, or a t.me link.`,
		Example: `  tg forward @friend --from @channel 100 101 102
  tg forward me --from @durov 12345`,
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args[1:])
			if err != nil {
				return err
			}

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				fromPeer, err := resolvePeer(ctx, m, from)
				if err != nil {
					return err
				}
				sender, err := a.sender(api)
				if err != nil {
					return err
				}

				fwd := builderFor(sender, args[0]).ForwardIDs(fromPeer, ids[0], ids[1:]...)
				if dropAuthor {
					fwd = fwd.DropAuthor()
				}
				if _, err := fwd.Send(ctx); err != nil {
					return errors.Wrap(err, "forward")
				}
				return a.printer.Emit(sentResult{Peer: args[0], MessageID: ids[len(ids)-1]})
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&from, "from", "", "source peer to forward from (required)")
	fs.BoolVar(&dropAuthor, "drop-author", false, "forward without quoting the original author")
	_ = cmd.MarkFlagRequired("from")

	return cmd
}
