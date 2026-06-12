package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// albumMedia builds an album media option from an uploaded file, choosing the
// kind from the MIME type. Caption (if any) is attached to this item.
func albumMedia(fileInput tg.InputFileClass, fileName, mime string, caption []styling.StyledTextOption) message.MultiMediaOption {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return message.UploadedPhoto(fileInput, caption...)
	case strings.HasPrefix(mime, "video/"):
		return message.UploadedDocument(fileInput, caption...).Filename(fileName).MIME(mime).Video()
	case strings.HasPrefix(mime, "audio/"):
		return message.UploadedDocument(fileInput, caption...).Filename(fileName).MIME(mime).Audio()
	default:
		return message.UploadedDocument(fileInput, caption...).Filename(fileName).MIME(mime).ForceFile(true)
	}
}

// uploadAlbumItem uploads a single file and returns its album media option.
func uploadAlbumItem(
	ctx context.Context,
	upld *uploader.Uploader,
	path string,
	caption []styling.StyledTextOption,
) (message.MultiMediaOption, error) {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 // user-provided path
	if err != nil {
		return nil, errors.Wrapf(err, "open %q", path)
	}
	defer func() { _ = f.Close() }()

	mime, err := detectMIME(f)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "stat %q", path)
	}

	fileName := filepath.Base(path)
	fileInput, err := upld.Upload(ctx, uploader.NewUpload(fileName, f, info.Size()))
	if err != nil {
		return nil, errors.Wrapf(err, "upload %q", path)
	}
	return albumMedia(fileInput, fileName, mime.String(), caption), nil
}

func (a *app) newAlbumCmd() *cobra.Command {
	var (
		peer    string
		caption string
	)

	cmd := &cobra.Command{
		Use:     "album <file> <file> [file...]",
		Short:   "Send multiple files as a grouped album",
		GroupID: groupMessaging,
		Long: `Upload two or more files and send them as a single grouped album. Photos,
videos and documents are supported (stickers are not). The caption, if any, is
attached to the first item.`,
		Example: `  tg album a.jpg b.jpg c.jpg
  tg album --peer @durov clip.mp4 photo.jpg --caption "trip"`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				upld := uploader.NewUploader(api).WithPartSize(uploader.MaximumPartSize)
				sender, err := a.sender(api)
				if err != nil {
					return err
				}
				sender = sender.WithUploader(upld)

				media := make([]message.MultiMediaOption, 0, len(args))
				for i, path := range args {
					var capt []styling.StyledTextOption
					if i == 0 && caption != "" {
						capt = []styling.StyledTextOption{styling.Plain(caption)}
					}
					item, err := uploadAlbumItem(ctx, upld, path, capt)
					if err != nil {
						return err
					}
					media = append(media, item)
				}

				id, err := unpack.MessageID(builderFor(sender, peer).Album(ctx, media[0], media[1:]...))
				if err != nil {
					return errors.Wrap(err, "send album")
				}
				return a.printer.Emit(sentResult{Peer: peer, MessageID: id})
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVarP(&peer, "peer", "p", "", "peer to send to; default: yourself")
	fs.StringVar(&caption, "caption", "", "caption attached to the first item")
	registerPeerCompletion(cmd, "peer")

	return cmd
}
