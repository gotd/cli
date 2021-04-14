package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gotd/td/clock"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func (p *app) uploadFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "peer",
			Aliases: []string{"p", "target"},
			Usage:   "Peer to write (e.g. channel name or username, phone number or deep link).",
		},
		&cli.StringFlag{
			Name:        "filename",
			Usage:       "Sets value of filename attribute. If empty, attribute will not be set.",
			DefaultText: "uses name from given path",
		},
		&EnumFlag{
			StringFlag: cli.StringFlag{
				Name:        "type",
				Aliases:     []string{"as"},
				Usage:       "Sets type of uploaded document.",
				DefaultText: "uses MIME type to detect",
			},
			Allowed: []string{"file", "video", "audio", "voice", "gif", "sticker"},
		},
		&cli.IntFlag{
			Name:    "threads",
			Aliases: []string{"j"},
			Value:   1,
			Usage:   "Concurrency",
		},
	}
}

func detectMIME(f io.ReadSeeker) (*mimetype.MIME, error) {
	mime, err := mimetype.DetectReader(f)
	if err != nil {
		return nil, fmt.Errorf("detect MIME: %w", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to start: %w", err)
	}

	return mime, nil
}

func prepareFile(c *cli.Context, fileInput tg.InputFileClass, fileName, mime string) message.MediaOption {
	file := message.UploadedDocument(fileInput, styling.Plain(fileName))
	if c.IsSet("filename") {
		if userFileName := c.String("filename"); userFileName != "" {
			file = file.Filename(userFileName)
		}
	} else {
		file = file.Filename(fileName)
	}

	if c.IsSet("type") {
		v := c.String("type")
		switch v {
		case "file":
			return file.ForceFile(true).MIME(mime)
		case "video":
			return file.Video()
		case "audio":
			return file.Audio()
		case "voice":
			return file.Voice()
		case "gif":
			return file.MIME(message.DefaultGifMIME).
				Attributes(&tg.DocumentAttributeAnimated{})
		case "sticker":
			return file.UploadedSticker()
		default:
			panic("unreachable: unknown type" + v)
		}
	}

	file = file.MIME(mime)
	switch {
	case strings.HasPrefix(mime, "video"):
		return file.Video()
	case strings.HasPrefix(mime, "audio"):
		return file.Audio()
	case mime == message.DefaultGifMIME:
		return file.Attributes(&tg.DocumentAttributeAnimated{})
	default:
		return file.ForceFile(true)
	}
}

func (p *app) uploadCmd(c *cli.Context) error {
	log := p.log

	return p.run(c.Context, func(ctx context.Context, api *tg.Client) error {
		arg := c.Args().First()
		if arg == "" {
			return errors.New("no file name provided")
		}

		upld := uploader.NewUploader(api).
			WithThreads(c.Int("threads")).
			WithPartSize(uploader.MaximumPartSize)
		sender := message.NewSender(api).WithUploader(upld)
		builder := sender.Self()
		if to := c.String("peer"); to != "" {
			builder = sender.Resolve(to)
		}

		return filepath.Walk(arg, func(path string, info fs.FileInfo, err error) error {
			// Stop if got error, skip if current file is directory.
			if err != nil || info.IsDir() {
				return err
			}

			f, err := os.Open(arg)
			if err != nil {
				return fmt.Errorf("open %q: %w", path, err)
			}
			defer func() {
				_ = f.Close()
			}()

			m, err := detectMIME(f)
			if err != nil {
				return err
			}

			fileName := filepath.Base(path)
			bar := progressbar.DefaultBytes(info.Size(), "upload "+fileName)
			upload := uploader.NewUpload(fileName, io.TeeReader(f, bar), info.Size())

			g, ctx := errgroup.WithContext(ctx)
			done := make(chan struct{})

			g.Go(func() error {
				sendProgress := func() {
					a := builder.TypingAction()
					percent := int(bar.State().CurrentPercent * 100)
					if err := a.UploadDocument(ctx, percent); err != nil && !errors.Is(err, context.Canceled) {
						log.Error("Action failed", zap.Error(err))
					}
				}

				// Initial progress.
				sendProgress()

				ticker := clock.System.Ticker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C():
						sendProgress()
					case <-ctx.Done():
						return ctx.Err()
					case <-done:
						return nil
					}
				}
			})

			g.Go(func() error {
				defer close(done)

				fileInput, err := upld.Upload(ctx, upload)
				if err != nil {
					return fmt.Errorf("upload %q: %w", path, err)
				}

				if _, err := builder.Media(ctx, prepareFile(c, fileInput, fileName, m.String())); err != nil {
					return fmt.Errorf("send %q: %w", path, err)
				}

				return nil
			})

			return g.Wait()
		})
	})
}
