package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
)

// accountStatus describes one configured account.
type accountStatus struct {
	Label      string `json:"label"`
	AppID      int    `json:"app_id"`
	HasBot     bool   `json:"has_bot"`
	HasSession bool   `json:"has_session"`
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
		if _, err := fmt.Fprintf(w, "%-16s app_id=%d %s\n", ac.Label, ac.AppID, session); err != nil {
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
			dir := filepath.Dir(a.configPath)
			var res accountsResult
			for _, label := range a.cfg.labels() {
				acc, err := a.cfg.account(label)
				if err != nil {
					return err
				}
				sessionPath := acc.sessionPath(dir, label, kindUser)
				_, statErr := os.Stat(sessionPath)
				res.Accounts = append(res.Accounts, accountStatus{
					Label:      label,
					AppID:      acc.AppID,
					HasBot:     acc.BotToken != "",
					HasSession: statErr == nil,
				})
			}
			return a.printer.Emit(res)
		},
	}

	cmd.AddCommand(a.newAccountsAddCmd(), a.newAccountsRemoveCmd())
	return cmd
}

func (a *app) newAccountsAddCmd() *cobra.Command {
	var (
		appID   int
		appHash string
		token   string
		proxy   string
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
			if appID == 0 || appHash == "" {
				return errors.New("--app-id and --app-hash are required")
			}
			if a.cfg.Accounts == nil {
				a.cfg.Accounts = map[string]Account{}
			}
			a.cfg.Accounts[label] = Account{
				AppID:    appID,
				AppHash:  appHash,
				BotToken: token,
				Proxy:    proxy,
				Test:     a.testServer, // persist the global --test flag
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
