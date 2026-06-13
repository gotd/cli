package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-faster/errors"
	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// termAuth implements auth.UserAuthenticator for the phone login flow, reading
// the phone/code/password from flags, env, or interactive prompts (on stderr).
type termAuth struct {
	phone    string
	password string
	in       *bufio.Reader
	out      io.Writer
}

func (t termAuth) prompt(label string) (string, error) {
	if _, err := fmt.Fprint(t.out, label); err != nil {
		return "", err
	}
	line, err := t.in.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (t termAuth) Phone(_ context.Context) (string, error) {
	if t.phone != "" {
		return t.phone, nil
	}
	return t.prompt("Phone (international, e.g. +123456789): ")
}

func (t termAuth) Password(_ context.Context) (string, error) {
	if t.password != "" {
		return t.password, nil
	}
	return t.prompt("2FA password: ")
}

func (t termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return t.prompt("Code (sent via Telegram): ")
}

func (termAuth) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign up is not supported; use an existing account")
}

func (termAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

// loginPhone runs the code + 2FA phone login flow.
func (a *app) loginPhone(ctx context.Context, client *telegram.Client, phone, password string) error {
	ua := termAuth{
		phone:    phone,
		password: password,
		in:       bufio.NewReader(os.Stdin),
		out:      os.Stderr,
	}
	flow := auth.NewFlow(ua, auth.SendCodeOptions{})
	if err := flow.Run(ctx, client.Auth()); err != nil {
		return errors.Wrap(err, "phone login")
	}
	return nil
}

// loginQR runs the QR login flow, falling back to 2FA on SESSION_PASSWORD_NEEDED.
func (a *app) loginQR(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher, password string) error {
	loggedIn := qrlogin.OnLoginToken(d)

	show := func(_ context.Context, token qrlogin.Token) error {
		fmt.Fprintln(os.Stderr, "Scan this QR in Telegram: Settings → Devices → Link Desktop Device")
		qrterminal.GenerateHalfBlock(token.URL(), qrterminal.L, os.Stderr)
		fmt.Fprintln(os.Stderr, "or open:", token.URL())
		return nil
	}

	if _, err := client.QR().Auth(ctx, loggedIn, show); err != nil {
		if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
			return a.complete2FA(ctx, client, password)
		}
		return errors.Wrap(err, "qr login")
	}
	return nil
}

// complete2FA submits the cloud password, re-prompting once on invalid password.
func (a *app) complete2FA(ctx context.Context, client *telegram.Client, password string) error {
	ua := termAuth{password: password, in: bufio.NewReader(os.Stdin), out: os.Stderr}
	for {
		pw, err := ua.Password(ctx)
		if err != nil {
			return err
		}
		if _, err := client.Auth().Password(ctx, pw); err != nil {
			if errors.Is(err, auth.ErrPasswordInvalid) && password == "" {
				fmt.Fprintln(os.Stderr, "Invalid password, try again.")
				continue
			}
			return errors.Wrap(err, "2fa password")
		}
		return nil
	}
}

// ensureAccount makes sure the selected account exists in the config before
// login, creating a default entry (built-in credentials) for a new named label.
// This lets `tg login --account <new>` work without a prior `tg accounts add`;
// the default account always exists, and `--account all` is rejected.
func (a *app) ensureAccount() error {
	if a.accountFlag == "all" {
		return errors.New("login needs a single --account, not 'all'")
	}
	label := a.accountFlag
	if label == "" {
		label = a.cfg.resolvedDefault()
	}
	if label == defaultAccount {
		return nil // the default account always exists
	}
	if _, ok := a.cfg.Accounts[label]; ok {
		return nil
	}
	if a.cfg.Accounts == nil {
		a.cfg.Accounts = map[string]Account{}
	}
	a.cfg.Accounts[label] = Account{}
	if err := saveConfig(a.configPath, a.cfg); err != nil {
		return err
	}
	_, err := fmt.Fprintf(os.Stderr, "Created account %q.\n", label)
	return err
}

func (a *app) newLoginCmd() *cobra.Command {
	var (
		phone    string
		password string
	)

	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Authenticate a personal user session",
		GroupID: groupAuth,
		Long: `Authenticate as a personal Telegram account and persist the session for
later headless use. QR login is the default: scan once from a logged-in device
(Settings → Devices → Link Desktop Device). Use --phone for a phone-code flow
where scanning is inconvenient. Two-factor (cloud password) is supported via
--password, the TG_PASSWORD env var, or an interactive prompt.

A new --account label is created on the fly (with built-in credentials), so you
don't need "tg accounts add" first; use that only to pre-set custom credentials,
a bot token, or a per-account proxy. Run "tg init" once before logging in.

The QR code and all prompts are written to stderr; the resulting account is
printed to stdout (honoring --output).`,
		Example: `  # QR login (default)
  tg login

  # Log in a new named account (created automatically)
  tg login --account work

  # Phone login
  tg login --phone +123456789

  # Non-interactive 2FA password
  TG_PASSWORD=secret tg login`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.ensureAccount(); err != nil {
				return err
			}
			if password == "" {
				password = os.Getenv("TG_PASSWORD")
			}
			usePhone := cmd.Flags().Changed("phone")
			phone = strings.TrimSpace(phone) // bare --phone (NoOptDefVal) trims to empty → prompt
			// QR login requires updates (the login-token signal); phone does not.
			rp := runParams{auth: authUser, updates: !usePhone}

			return a.connect(cmd.Context(), rp, func(ctx context.Context, client *telegram.Client, d tg.UpdateDispatcher) error {
				status, err := client.Auth().Status(ctx)
				if err != nil {
					return errors.Wrap(err, "auth status")
				}
				if status.Authorized {
					fmt.Fprintln(os.Stderr, "Already authorized.")
				} else {
					if usePhone {
						err = a.loginPhone(ctx, client, phone, password)
					} else {
						err = a.loginQR(ctx, client, d, password)
					}
					if err != nil {
						return err
					}
					fmt.Fprintln(os.Stderr, "Login successful; session saved.")
				}

				user, err := fetchSelf(ctx, client.API())
				if err != nil {
					return err
				}
				return a.printer.Emit(newWhoamiResult(user))
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&phone, "phone", "", "phone-code login with this number (international format; use --phone= to be prompted)")
	fs.StringVar(&password, "password", "", "2FA cloud password (or set TG_PASSWORD)")

	return cmd
}
