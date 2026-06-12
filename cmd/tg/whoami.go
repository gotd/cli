package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
)

// whoamiResult is the stable result of `tg whoami`.
type whoamiResult struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Bot       bool   `json:"bot"`
}

func newWhoamiResult(u *tg.User) whoamiResult {
	r := whoamiResult{
		ID:        u.ID,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
		Bot:       u.Bot,
	}
	return r
}

// MarshalText renders a human-readable line.
func (r whoamiResult) MarshalText(w io.Writer) error {
	name := strings.TrimSpace(r.FirstName + " " + r.LastName)
	parts := []string{fmt.Sprintf("id=%d", r.ID)}
	if name != "" {
		parts = append(parts, fmt.Sprintf("name=%q", name))
	}
	if r.Username != "" {
		parts = append(parts, "username=@"+r.Username)
	}
	if r.Phone != "" {
		parts = append(parts, "phone=+"+r.Phone)
	}
	if r.Bot {
		parts = append(parts, "bot=true")
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, " "))
	return err
}

// fetchSelf returns the authenticated account's own user.
func fetchSelf(ctx context.Context, api *tg.Client) (*tg.User, error) {
	users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err != nil {
		return nil, errors.Wrap(err, "users.getUsers")
	}
	if len(users) == 0 {
		return nil, errors.New("empty users response")
	}
	user, ok := users[0].AsNotEmpty()
	if !ok {
		return nil, errors.Errorf("unexpected user type %T", users[0])
	}
	return user, nil
}

// runWhoami fetches the current account and emits it. It takes only an API
// client and printer so it is unit-testable with a mock invoker.
func runWhoami(ctx context.Context, api *tg.Client, p *output.Printer) error {
	user, err := fetchSelf(ctx, api)
	if err != nil {
		return err
	}
	return p.Emit(newWhoamiResult(user))
}

func (a *app) newWhoamiCmd() *cobra.Command {
	var asBot bool

	cmd := &cobra.Command{
		Use:     "whoami",
		Short:   "Show the authenticated account",
		GroupID: groupAuth,
		Long: `Print the account behind the current session: ID, name, username, phone.
Useful to confirm authentication and as a minimal end-to-end check.`,
		Example: `  tg whoami
  tg whoami --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			auth := authUser
			if asBot {
				auth = authBot
			}
			return a.run(cmd.Context(), runParams{auth: auth}, func(ctx context.Context, api *tg.Client) error {
				return runWhoami(ctx, api, a.printer)
			})
		},
	}

	cmd.Flags().BoolVar(&asBot, kindBot, false, "use the bot session instead of the user session")

	return cmd
}
