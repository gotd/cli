package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func (a *app) newLogoutCmd() *cobra.Command {
	var asBot bool

	cmd := &cobra.Command{
		Use:     "logout",
		Short:   "Log out and remove the local session",
		GroupID: groupAuth,
		Long: `Invalidate the session on Telegram's side and delete the local session and
peer cache for the selected account. Remote logout is best-effort: the local
files are removed even if it fails (e.g. the session is already invalid).`,
		Example: "  tg logout\n  tg logout --account work\n  tg logout --bot",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kind := authUser
			if asBot {
				kind = authBot
			}
			if err := a.ensureActive(); err != nil {
				return err
			}
			st := a.active
			dir := filepath.Dir(a.configPath)
			store := a.sessionStore(st.label, st.acc, kind.String())
			cachePath := st.acc.peerCachePath(dir, st.label, kind.String())

			// Best-effort server-side logout.
			err := a.connectWith(cmd.Context(), st, runParams{auth: kind},
				func(ctx context.Context, client *telegram.Client, _ tg.UpdateDispatcher) error {
					status, err := client.Auth().Status(ctx)
					if err != nil {
						return errors.Wrap(err, "auth status")
					}
					if !status.Authorized {
						return nil
					}
					if _, err := client.API().AuthLogOut(ctx); err != nil {
						return errors.Wrap(err, "auth.logOut")
					}
					return nil
				})
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Warning: remote logout failed, removing local session anyway:", err)
			}

			// Remove the local session and peer cache regardless.
			if rmErr := store.Delete(cmd.Context()); rmErr != nil {
				return errors.Wrap(rmErr, "remove session")
			}
			if rmErr := os.Remove(cachePath); rmErr != nil && !os.IsNotExist(rmErr) {
				return errors.Wrapf(rmErr, "remove %s", cachePath)
			}
			return a.printer.Emit(okResult{OK: true})
		},
	}

	cmd.Flags().BoolVar(&asBot, kindBot, false, "log out the bot session instead of the user session")

	return cmd
}
