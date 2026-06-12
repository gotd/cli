package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

// downloadResult is the result of `tg download`.
type downloadResult struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// MarshalText renders a short summary.
func (r downloadResult) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "downloaded %s (%d bytes)\n", r.Path, r.Size)
	return err
}

// getMessage fetches a single message by id from peer.
func getMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, id int) (*tg.Message, error) {
	var (
		res tg.MessagesMessagesClass
		err error
	)
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		res, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: id}},
		})
	} else {
		res, err = api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: id}})
	}
	if err != nil {
		return nil, errors.Wrap(err, "get messages")
	}

	msgs, ok := res.AsModified()
	if !ok {
		return nil, errors.Errorf("unexpected messages type %T", res)
	}
	for _, m := range msgs.GetMessages() {
		if msg, ok := m.(*tg.Message); ok && msg.ID == id {
			return msg, nil
		}
	}
	return nil, errors.Errorf("message #%d not found", id)
}

// documentFilename returns the filename attribute of a document, or a fallback.
func documentFilename(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		if name, ok := attr.(*tg.DocumentAttributeFilename); ok && name.FileName != "" {
			return name.FileName
		}
	}
	return "document_" + strconv.FormatInt(doc.ID, 10)
}

// largestPhotoSize returns the type of the largest available photo size.
func largestPhotoSize(p *tg.Photo) (string, bool) {
	var (
		bestType string
		bestArea int
		found    bool
	)
	for _, s := range p.Sizes {
		switch v := s.(type) {
		case *tg.PhotoSize:
			if area := v.W * v.H; area >= bestArea {
				bestArea, bestType, found = area, v.Type, true
			}
		case *tg.PhotoSizeProgressive:
			if area := v.W * v.H; area >= bestArea {
				bestArea, bestType, found = area, v.Type, true
			}
		}
	}
	return bestType, found
}

// mediaLocation extracts a downloadable file location and default filename from
// message media.
func mediaLocation(m tg.MessageMediaClass) (tg.InputFileLocationClass, string, error) {
	switch v := m.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := v.Document.AsNotEmpty()
		if !ok {
			return nil, "", errors.New("empty document")
		}
		return doc.AsInputDocumentFileLocation(), documentFilename(doc), nil
	case *tg.MessageMediaPhoto:
		ph, ok := v.Photo.AsNotEmpty()
		if !ok {
			return nil, "", errors.New("empty photo")
		}
		sizeType, ok := largestPhotoSize(ph)
		if !ok {
			return nil, "", errors.New("no downloadable photo size")
		}
		loc := &tg.InputPhotoFileLocation{
			ID:            ph.ID,
			AccessHash:    ph.AccessHash,
			FileReference: ph.FileReference,
			ThumbSize:     sizeType,
		}
		return loc, "photo_" + strconv.FormatInt(ph.ID, 10) + ".jpg", nil
	default:
		return nil, "", errors.Errorf("message has no downloadable media (%T)", m)
	}
}

// resolveOutPath decides the destination path given the user's --out and the
// media's default filename.
func resolveOutPath(out, defaultName string) string {
	if out == "" {
		return defaultName
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return filepath.Join(out, defaultName)
	}
	return out
}

func (a *app) newDownloadCmd() *cobra.Command {
	var out string

	cmd := &cobra.Command{
		Use:     "download <peer> <message-id>",
		Aliases: []string{"dl"},
		Short:   "Download media from a message",
		GroupID: groupMessaging,
		Long: `Download the media (document, video, audio, photo, ...) attached to a
message. --out may be a file path or a directory; by default the media's own
filename is used in the current directory.`,
		Example: `  tg download @durov 12345
  tg download me 1000 --out ./downloads/
  tg download @channel 42 --out file.bin`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return errors.Wrap(err, "message-id must be an integer")
			}

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}

				msg, err := getMessage(ctx, api, peer, id)
				if err != nil {
					return err
				}
				media, ok := msg.GetMedia()
				if !ok {
					return errors.Errorf("message #%d has no media", id)
				}
				loc, name, err := mediaLocation(media)
				if err != nil {
					return err
				}
				path := resolveOutPath(out, name)

				if _, err := downloader.NewDownloader().Download(api, loc).ToPath(ctx, path); err != nil {
					return errors.Wrap(err, "download")
				}

				info, err := os.Stat(path)
				if err != nil {
					return errors.Wrap(err, "stat downloaded file")
				}
				return a.printer.Emit(downloadResult{Path: path, Size: info.Size()})
			})
		},
	}

	cmd.Flags().StringVar(&out, "out", "", "output file or directory (default: media filename in cwd)")
	_ = cmd.MarkFlagFilename("out")

	return cmd
}
