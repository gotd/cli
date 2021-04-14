package main

import (
	"context"
	"time"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"
	"github.com/urfave/cli/v2"
)

func (p *app) sendFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "peer",
			Aliases: []string{"p", "target"},
			Usage:   "Peer to write (e.g. channel name or username, phone number or deep link).",
		},
		&cli.BoolFlag{
			Name:  "html",
			Usage: "Use HTML styling.",
		},
		&cli.BoolFlag{
			Name:  "silent",
			Usage: "Sets flag to send this message silently (no notifications for the receivers).",
		},
		&cli.BoolFlag{
			Name:  "nowebpage",
			Usage: "Sets flag to disable generation of the webpage preview.",
		},
		&cli.DurationFlag{
			Name:  "schedule",
			Usage: "Sets scheduled message date for scheduled messages.",
		},
	}
}

func (p *app) sendCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, api *tg.Client) error {
		sender := message.NewSender(api)
		r := sender.Self()
		if targetDomain := c.String("peer"); targetDomain != "" {
			r = sender.Resolve(targetDomain)
		}
		p := &r.Builder

		if c.Bool("silent") {
			p = p.Silent()
		}

		if c.Bool("nowebpage") {
			p = p.NoWebpage()
		}

		if d := c.Duration("schedule"); c.IsSet("schedule") {
			p = p.Schedule(time.Now().Add(d))
		}

		if c.Bool("html") {
			if _, err := p.StyledText(ctx, html.String(c.Args().First())); err != nil {
				return err
			}
		} else {
			if _, err := p.Text(ctx, c.Args().First()); err != nil {
				return err
			}
		}

		return nil
	})
}
