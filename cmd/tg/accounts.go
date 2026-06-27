package main

import (
	"fmt"
	"io"
	"os"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
)

// accountStatus describes one configured account.
type accountStatus struct {
	Label      string `json:"label"`
	AppID      int    `json:"app_id"`
	HasBot     bool   `json:"has_bot"`
	HasSession bool   `json:"has_session"`
	Default    bool   `json:"default"`
}

// accountsResult is the result of `tg accounts`.
type accountsResult struct {
	Accounts []accountStatus `json:"accounts"`
}

// MarshalText renders one account per line.
func (r accountsResult) MarshalText(w io.Writer) error {
	for _, ac := range r.Accounts {
		session := "no-session"
		if ac.HasSession {
			session = "session"
		}
		marker := " "
		if ac.Default {
			marker = "*"
		}
		if _, err := fmt.Fprintf(w, "%s %-16s app_id=%d %s\n", marker, ac.Label, ac.AppID, session); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) newAccountsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "accounts",
		Short:   "List and manage configured accounts",
		GroupID: groupAuth,
		Long:    "List configured accounts (with auth status), or add/remove named accounts.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			def := a.cfg.resolvedDefault()
			var res accountsResult
			for _, label := range a.cfg.labels() {
				acc, err := a.cfg.account(label)
				if err != nil {
					return err
				}
				hasSession, err := a.sessionStore(label, acc, kindUser).Exists(cmd.Context())
				if err != nil {
					return errors.Wrapf(err, "check session for %s", label)
				}
				res.Accounts = append(res.Accounts, accountStatus{
					Label:      label,
					AppID:      acc.AppID,
					HasBot:     acc.BotToken != "",
					HasSession: hasSession,
					Default:    label == def,
				})
			}
			return a.printer.Emit(res)
		},
	}

	cmd.AddCommand(a.newAccountsAddCmd(), a.newAccountsRemoveCmd(), a.newAccountsDefaultCmd())
	return cmd
}

func (a *app) newAccountsDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default [label]",
		Short: "Show or set the default account",
		Long: `With no argument, print the current default account. With a label, make
that account the default used when --account / TG_ACCOUNT is not set.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), a.cfg.resolvedDefault())
				return err
			}
			label := args[0]
			if _, err := a.cfg.account(label); err != nil {
				return err
			}
			a.cfg.DefaultAccount = label
			if err := saveConfig(a.configPath, a.cfg); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Default account set to %q\n", label)
			return err
		},
	}
	return cmd
}

func (a *app) newAccountsAddCmd() *cobra.Command {
	var (
		appID   int
		appHash string
		token   string
		proxy   string
		test    bool
	)

	cmd := &cobra.Command{
		Use:   "add <label>",
		Short: "Add or update a named account",
		Long:  "Add a named account to the config. Then run: tg login --account <label>.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[0]
			if label == defaultAccount {
				return errors.New("use tg init for the default account")
			}
			if a.cfg.Accounts == nil {
				a.cfg.Accounts = map[string]Account{}
			}
			a.cfg.Accounts[label] = Account{
				AppID:    appID,
				AppHash:  appHash,
				BotToken: token,
				Proxy:    proxy,
				Test:     test,
			}
			if err := saveConfig(a.configPath, a.cfg); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Added account %q\n", label)
			return err
		},
	}

	fs := cmd.Flags()
	fs.IntVar(&appID, "app-id", envInt("APP_ID"), "telegram app ID")
	fs.StringVar(&appHash, "app-hash", os.Getenv("APP_HASH"), "telegram app hash")
	fs.StringVar(&token, "token", "", "optional bot token")
	fs.StringVar(&proxy, "proxy", "", "optional per-account proxy URL")
	fs.BoolVar(&test, "test", false, "create the account against the telegram test server")

	return cmd
}

func (a *app) newAccountsRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <label>",
		Short: "Remove a named account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[0]
			if _, ok := a.cfg.Accounts[label]; !ok {
				return errors.Errorf("unknown account %q", label)
			}
			delete(a.cfg.Accounts, label)
			if err := saveConfig(a.configPath, a.cfg); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Removed account %q\n", label)
			return err
		},
	}
	return cmd
}
