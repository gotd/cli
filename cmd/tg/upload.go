package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-faster/errors"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/gotd/td/clock"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
)

// uploadFlags are the parsed flag values for the upload command.
type uploadFlags struct {
	peer     string
	message  string
	filename string
	docType  *enumValue
	threads  int
	msg      messageOptions
}

// uploadedFile is one uploaded file in an uploadResult.
type uploadedFile struct {
	Path      string `json:"path"`
	MessageID int    `json:"message_id"`
}

// uploadResult is the result of `tg upload`.
type uploadResult struct {
	Files []uploadedFile `json:"files"`
}

// MarshalText renders one uploaded file per line.
func (r uploadResult) MarshalText(w io.Writer) error {
	for _, f := range r.Files {
		if _, err := fmt.Fprintf(w, "uploaded %s as message #%d\n", f.Path, f.MessageID); err != nil {
			return err
		}
	}
	return nil
}

// documentTypes are the allowed values of the --type flag.
func documentTypes() []string {
	return []string{"file", "video", "audio", "voice", "gif", "sticker"}
}

func detectMIME(f io.ReadSeeker) (*mimetype.MIME, error) {
	mime, err := mimetype.DetectReader(f)
	if err != nil {
		return nil, errors.Wrap(err, "detect MIME")
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Wrap(err, "seek to start")
	}

	return mime, nil
}

func prepareFile(
	uf *uploadFlags,
	fileInput tg.InputFileClass,
	option []styling.StyledTextOption,
	fileName, mime string,
) message.MediaOption {
	file := message.UploadedDocument(fileInput, option...)
	if uf.filename != "" {
		file = file.Filename(uf.filename)
	} else {
		file = file.Filename(fileName)
	}

	if v := uf.docType.value; v != "" {
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

func (a *app) updateStatus(
	ctx context.Context,
	done chan struct{},
	builder *message.RequestBuilder,
	bar *progressbar.ProgressBar,
) error {
	sendProgress := func() {
		act := builder.TypingAction()
		percent := int(bar.State().CurrentPercent * 100)
		if err := act.UploadDocument(ctx, percent); err != nil && !errors.Is(err, context.Canceled) {
			a.log.Error("Action failed", zap.Error(err))
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
}

func (a *app) uploadOne(
	ctx context.Context,
	uf *uploadFlags,
	upld *uploader.Uploader,
	builder *message.RequestBuilder,
	path string,
	info fs.FileInfo,
	quiet bool,
) (int, error) {
	f, err := os.Open(filepath.Clean(path)) // #nosec G304 // path comes from user-provided upload target
	if err != nil {
		return 0, errors.Wrapf(err, "open %q", path)
	}
	defer func() {
		_ = f.Close()
	}()

	m, err := detectMIME(f)
	if err != nil {
		return 0, err
	}

	fileName := filepath.Base(path)

	uploadFileName := fileName
	if uf.filename != "" {
		uploadFileName = uf.filename
	}

	// In machine mode, suppress the progress bar and typing action; otherwise
	// render a progress bar to stderr and report a typing action.
	var reader io.Reader = f
	var bar *progressbar.ProgressBar
	if !quiet {
		bar = progressbar.DefaultBytes(info.Size(), "upload "+fileName)
		reader = io.TeeReader(f, bar)
	}
	upload := uploader.NewUpload(uploadFileName, reader, info.Size())

	g, ctx := errgroup.WithContext(ctx)
	done := make(chan struct{})
	if bar != nil {
		g.Go(func() error {
			return a.updateStatus(ctx, done, builder, bar)
		})
	}

	var msgID int
	g.Go(func() error {
		defer close(done)

		fileInput, err := upld.Upload(ctx, upload)
		if err != nil {
			return errors.Wrapf(err, "upload %q", path)
		}

		b, options := uf.msg.apply(builder, uf.message)
		id, err := unpack.MessageID(b.Media(ctx, prepareFile(uf, fileInput, options, fileName, m.String())))
		if err != nil {
			return errors.Wrapf(err, "send %q", path)
		}
		msgID = id
		return nil
	})

	if err := g.Wait(); err != nil {
		return 0, err
	}
	return msgID, nil
}

func (a *app) newUploadCmd() *cobra.Command {
	uf := &uploadFlags{docType: newEnumValue("", documentTypes()...)}

	cmd := &cobra.Command{
		Use:     "upload [flags] <path>",
		Aliases: []string{"up"},
		Short:   "Upload a file (or directory of files) to a peer",
		GroupID: groupMessaging,
		Long: `Upload a file to a peer. If <path> is a directory, every file under it is
uploaded. With no --peer, files go to your own Saved Messages.

The document type is detected from the file's MIME type unless --type is set.`,
		Example: `  # Upload a file to Saved Messages
  tg upload ./report.pdf

  # Upload as a video to a chat
  tg upload --peer @me --type video clip.mp4

  # Upload with a caption
  tg upload --peer @durov --message "here you go" photo.jpg`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := args[0]

			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				upld := uploader.NewUploader(api).
					WithThreads(uf.threads).
					WithPartSize(uploader.MaximumPartSize)
				sender, err := a.sender(api)
				if err != nil {
					return err
				}
				sender = sender.WithUploader(upld)

				builder := builderFor(sender, uf.peer)
				quiet := a.printer.Format() == output.JSON

				var result uploadResult
				if err := filepath.Walk(arg, func(path string, info fs.FileInfo, err error) error {
					// Stop if got error, skip if current file is directory.
					if info.IsDir() || err != nil {
						return err
					}
					id, err := a.uploadOne(ctx, uf, upld, builder, path, info, quiet)
					if err != nil {
						return err
					}
					result.Files = append(result.Files, uploadedFile{Path: path, MessageID: id})
					return nil
				}); err != nil {
					return err
				}
				return a.printer.Emit(result)
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&uf.peer, "peer", "p", "",
		"peer to write (channel name, username, phone number or deep link); default: yourself")
	flags.StringVarP(&uf.message, "message", "m", "", "text message (caption) to send with the file")
	flags.StringVar(&uf.filename, "filename", "",
		"value of the filename attribute (defaults to the name from the given path)")
	flags.Var(uf.docType, "type", "type of uploaded document (default: detect from MIME)")
	flags.IntVarP(&uf.threads, "threads", "j", 1, "concurrency limit")
	uf.msg.register(flags)

	registerPeerCompletion(cmd, "peer")
	registerEnumCompletion(cmd, "type", documentTypes())

	return cmd
}
