package main

import (
	"context"

	"github.com/urfave/cli/v2"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func (p *app) sendFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringFlag{
			Name:    "peer",
			Aliases: []string{"p", "target"},
			Usage:   "peer to write (e.g. channel name or username, phone number or deep link)",
		},
	}, messageFlags()...)
}

func (p *app) sendCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, api *tg.Client) error {
		sender := message.NewSender(api)

		builder := sender.Self()
		if targetDomain := c.String("peer"); targetDomain != "" {
			builder = sender.Resolve(targetDomain)
		}

		b, options := applyMessageFlags(c, builder, c.Args().First())
		if _, err := b.StyledText(ctx, options...); err != nil {
			return err
		}

		return nil
	})
}
