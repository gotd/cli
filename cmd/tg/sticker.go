package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func (p *app) stickerCmdFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "sticker-set",
			Aliases:  []string{"set", "name"},
			Required: true,
			Usage:    "name of sticker set to add sticker (from tg://addstickers?set=short_name)",
		},
	}
}

func (p *app) stickerAddFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringFlag{
			Name:     "emoji",
			Aliases:  []string{"e"},
			Required: true,
			Usage:    "emoji list to associate with sticker",
		},
	}, p.stickerCmdFlags()...)
}

func uploadSticker(ctx context.Context, api *tg.Client, arg string) (*tg.Document, error) {
	upld := uploader.NewUploader(api).
		WithPartSize(uploader.MaximumPartSize)

	file, err := upld.FromPath(ctx, arg)
	if err != nil {
		return nil, xerrors.Errorf("upload sticker: %w", err)
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
		return nil, xerrors.Errorf("confirm upload: %w", err)
	}

	// TODO: return and print pretty error if uploaded file has invalid type
	media, ok := confirmed.(*tg.MessageMediaDocument)
	if !ok {
		return nil, xerrors.Errorf("unexpected media type %T", confirmed)
	}

	doc, ok := media.Document.AsNotEmpty()
	if !ok {
		return nil, xerrors.Errorf("invalid document %T", media.Document)
	}

	return doc, nil
}

func (p *app) stickerAddCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()

		arg := c.Args().First()
		if arg == "" {
			return errors.New("no file name provided")
		}

		doc, err := uploadSticker(ctx, api, arg)
		if err != nil {
			return err
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
		fmt.Printf("Successfully added sticker to set @%s\n", set.Set.ShortName)

		return nil
	})
}

func (p *app) stickerCreateFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringFlag{
			Name:     "title",
			Aliases:  []string{"t"},
			Required: true,
			Usage:    "Sticker set title, 1-64 chars",
		},
		&cli.StringFlag{
			Name:     "owner",
			Aliases:  []string{"o"},
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
	}, p.stickerAddFlags()...)
}

func (p *app) stickerCreateCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		sender := message.NewSender(api)

		arg := c.Args().First()
		if arg == "" {
			return errors.New("no file name provided")
		}

		owner, err := sender.Resolve(c.String("owner")).AsInputUser(ctx)
		if err != nil {
			return xerrors.Errorf("resolve owner: %w", err)
		}

		doc, err := uploadSticker(ctx, api, arg)
		if err != nil {
			return err
		}

		set, err := api.StickersCreateStickerSet(ctx, &tg.StickersCreateStickerSetRequest{
			Masks:    c.Bool("masks"),
			Animated: c.Bool("animated"),
			UserID:   owner,
			Title:    c.String("title"),
			// It must begin with a letter, can’t contain consecutive underscores and must end in ‘_by_<bot username>’.
			// See https://docs.pyrogram.org/api/errors/bad-request#:~:text=PACK_SHORT_NAME_INVALID
			ShortName: c.String("sticker-set"),
			Stickers: []tg.InputStickerSetItem{
				{
					Document: doc.AsInput(),
					Emoji:    c.String("emoji"),
				},
			},
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

func (p *app) stickerRemoveFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.IntFlag{
			Name:     "n",
			Required: true,
			Usage:    "Index of sticker to delete",
		},
		&cli.StringFlag{
			Name:  "download",
			Usage: "Output path to download sticker before deletion",
		},
	}, p.stickerCmdFlags()...)
}

func (p *app) stickerRemoveCmd(c *cli.Context) error {
	return p.run(c.Context, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()

		set, err := api.MessagesGetStickerSet(ctx, &tg.InputStickerSetShortName{
			ShortName: c.String("sticker-set"),
		})
		if err != nil {
			return xerrors.Errorf("get sticker set: %w", err)
		}

		idx := c.Int("n")
		if length := len(set.Documents); idx < 0 || length <= idx {
			return xerrors.Errorf("index is too big, there are only %d stickers", length)
		}

		doc, ok := set.Documents[idx].AsNotEmpty()
		if !ok {
			return xerrors.Errorf("unexpected document type %T", set.Documents[idx])
		}

		if path := c.String("download"); c.IsSet("download") {
			if _, err := downloader.NewDownloader().
				Download(api, doc.AsInputDocumentFileLocation()).
				ToPath(ctx, path); err != nil {
				return xerrors.Errorf("download to %q: %w", path, err)
			}
		}

		_, err = api.StickersRemoveStickerFromSet(ctx, doc.AsInput())
		if err != nil {
			return xerrors.Errorf("delete sticker: %w", err)
		}
		fmt.Printf(
			"Successfully remove sticker from set @%s\n",
			set.Set.ShortName,
		)

		return nil
	})
}
