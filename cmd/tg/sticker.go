package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func (p *app) stickerAddFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "sticker-set",
			Required: true,
			Usage:    "name of sticker set to add sticker (from tg://addstickers?set=short_name)",
		},
		&cli.StringFlag{
			Name:     "emoji",
			Required: true,
			Usage:    "emoji list to associate with sticker",
		},
	}
}

func (p *app) stickerAddCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, api *tg.Client) error {
		arg := c.Args().First()
		if arg == "" {
			return errors.New("no file name provided")
		}

		upld := uploader.NewUploader(api).
			WithPartSize(uploader.MaximumPartSize)

		file, err := upld.FromPath(ctx, arg)
		if err != nil {
			return xerrors.Errorf("upload sticker: %w", err)
		}

		// TODO: replace with some helper
		confirmed, err := api.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
			Peer: &tg.InputPeerSelf{},
			Media: &tg.InputMediaUploadedDocument{
				File:     file,
				MimeType: "image/png",
			},
		})
		if err != nil {
			return xerrors.Errorf("confirm upload: %w", err)
		}

		// TODO: return and print pretty error if uploaded file has invalid type
		media, ok := confirmed.(*tg.MessageMediaDocument)
		if !ok {
			return xerrors.Errorf("unexpected media type %T", confirmed)
		}

		doc, ok := media.Document.AsNotEmpty()
		if !ok {
			return xerrors.Errorf("invalid document %T", media.Document)
		}

		set, err := api.StickersAddStickerToSet(ctx, &tg.StickersAddStickerToSetRequest{
			Stickerset: &tg.InputStickerSetShortName{ShortName: c.String("sticker-set")},
			Sticker: tg.InputStickerSetItem{
				Document: doc.AsInput(),
				Emoji:    c.String("emoji"),
			},
		})
		if err != nil {
			return xerrors.Errorf("add to sticker set: %w", err)
		}
		fmt.Printf("Successfully added to sticker set @%s\n", set.Set.ShortName)

		return nil
	})
}

func (p *app) stickerCreateFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "title",
			Required: true,
			Usage:    "Sticker set title, 1-64 chars",
		},
		&cli.StringFlag{
			Name:     "owner",
			Required: true,
			Usage:    "Sticker set owner username",
		},
		&cli.BoolFlag{
			Name:  "masks",
			Usage: "Whether this is a mask sticker set",
		},
		&cli.BoolFlag{
			Name:  "animated",
			Usage: "Whether this is a animated sticker set",
		},
	}
}

func (p *app) stickerCreateCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, api *tg.Client) error {
		sender := message.NewSender(api)

		arg := c.Args().First()
		if arg == "" {
			return errors.New("no sticker set shortname provided")
		}

		builder := sender.Self()
		if targetDomain := c.String("owner"); targetDomain != "" {
			builder = sender.Resolve(targetDomain)
		}

		owner, err := builder.AsInputUser(ctx)
		if err != nil {
			return xerrors.Errorf("resolve owner: %w", err)
		}

		set, err := api.StickersCreateStickerSet(ctx, &tg.StickersCreateStickerSetRequest{
			Masks:     c.Bool("masks"),
			Animated:  c.Bool("animated"),
			UserID:    owner,
			Title:     c.String("title"),
			ShortName: arg,
		})
		if err != nil {
			return xerrors.Errorf("create set: %w", err)
		}

		fmt.Printf(
			"Successfully created sticker set @%s\nTo add: https://t.me/addstickers/%[1]s",
			set.Set.ShortName,
		)

		return nil
	})
}
