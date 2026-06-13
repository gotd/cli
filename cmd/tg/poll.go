package main

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
)

func (a *app) newPollCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "poll",
		Short:   "Create polls",
		GroupID: groupMessaging,
		Long:    "Commands for working with polls.",
	}
	cmd.AddCommand(a.newPollCreateCmd())
	return cmd
}

func (a *app) newPollCreateCmd() *cobra.Command {
	var (
		multiple bool
		public   bool
		closeIn  time.Duration
	)

	cmd := &cobra.Command{
		Use:   "create <peer> <question> <option> <option> [option...]",
		Short: "Create a poll",
		Long: `Create a poll with a question and at least two options. By default the
poll is single-choice and anonymous.`,
		Example: `  tg poll create @group "Lunch?" "Pizza" "Sushi" "Salad"
  tg poll create me "Pick" "A" "B" --multiple --public --close-in 1h`,
		Args:              cobra.MinimumNArgs(4),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			peer, question, options := args[0], args[1], args[2:]

			answers := make([]message.PollAnswerOption, 0, len(options))
			for _, o := range options {
				answers = append(answers, message.PollAnswer(o))
			}

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				sender, m, err := a.sender(api)
				if err != nil {
					return err
				}
				bf, err := builderFor(ctx, m, sender, peer)
				if err != nil {
					return err
				}

				poll := message.Poll(question, answers[0], answers[1], answers[2:]...)
				if multiple {
					poll = poll.MultipleChoice(true)
				}
				if public {
					poll = poll.PublicVoters(true)
				}
				if closeIn > 0 {
					poll = poll.ClosePeriod(closeIn)
				}

				id, err := unpack.MessageID(bf.Media(ctx, poll))
				if err != nil {
					return errors.Wrap(err, "create poll")
				}
				return a.printer.Emit(sentResult{Peer: peer, MessageID: id})
			})
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&multiple, "multiple", false, "allow multiple answers")
	fs.BoolVar(&public, "public", false, "make votes public (non-anonymous)")
	fs.DurationVar(&closeIn, "close-in", 0, "automatically close the poll after this duration")

	return cmd
}
