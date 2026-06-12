package main

import (
	"context"
	"path/filepath"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

const groupProfile = "profile"

func (a *app) newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profile",
		Short:   "Manage your profile",
		GroupID: groupProfile,
		Long:    "Update your profile, set or delete your profile photo, and set your status.",
	}
	cmd.AddCommand(
		a.newProfileUpdateCmd(),
		a.newProfileSetPhotoCmd(),
		a.newProfileDeletePhotoCmd(),
		a.newProfileStatusCmd(),
	)
	return cmd
}

func (a *app) newProfileUpdateCmd() *cobra.Command {
	var first, last, about string

	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Update your name and bio",
		Args:    cobra.NoArgs,
		Example: `  tg profile update --first-name Ada --last-name Lovelace --about "math"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			req := &tg.AccountUpdateProfileRequest{}
			if cmd.Flags().Changed("first-name") {
				req.SetFirstName(first)
			}
			if cmd.Flags().Changed("last-name") {
				req.SetLastName(last)
			}
			if cmd.Flags().Changed("about") {
				req.SetAbout(about)
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				if _, err := api.AccountUpdateProfile(ctx, req); err != nil {
					return errors.Wrap(err, "account.updateProfile")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&first, "first-name", "", "first name")
	fs.StringVar(&last, "last-name", "", "last name")
	fs.StringVar(&about, "about", "", "bio / about text")

	return cmd
}

func (a *app) newProfileSetPhotoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-photo <file>",
		Short: "Set your profile photo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				file, err := uploader.NewUploader(api).FromPath(ctx, filepath.Clean(args[0]))
				if err != nil {
					return errors.Wrapf(err, "upload %q", args[0])
				}
				if _, err := api.PhotosUploadProfilePhoto(ctx, &tg.PhotosUploadProfilePhotoRequest{File: file}); err != nil {
					return errors.Wrap(err, "photos.uploadProfilePhoto")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newProfileDeletePhotoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-photo",
		Short: "Delete your current profile photo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.PhotosGetUserPhotos(ctx, &tg.PhotosGetUserPhotosRequest{
					UserID: &tg.InputUserSelf{},
					Limit:  1,
				})
				if err != nil {
					return errors.Wrap(err, "photos.getUserPhotos")
				}
				photos := res.GetPhotos()
				if len(photos) == 0 {
					return errors.New("no profile photo to delete")
				}
				photo, ok := photos[0].AsNotEmpty()
				if !ok {
					return errors.New("current profile photo is empty")
				}
				if _, err := api.PhotosDeletePhotos(ctx, []tg.InputPhotoClass{&tg.InputPhoto{
					ID:            photo.ID,
					AccessHash:    photo.AccessHash,
					FileReference: photo.FileReference,
				}}); err != nil {
					return errors.Wrap(err, "photos.deletePhotos")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
	return cmd
}

func (a *app) newProfileStatusCmd() *cobra.Command {
	var offline bool

	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Set your online/offline status",
		Args:    cobra.NoArgs,
		Example: "  tg profile status            # mark online\n  tg profile status --offline",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				if _, err := api.AccountUpdateStatus(ctx, offline); err != nil {
					return errors.Wrap(err, "account.updateStatus")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}

	cmd.Flags().BoolVar(&offline, "offline", false, "mark as offline (default: online)")

	return cmd
}
