package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Command groups shown in `tg --help`.
const (
	groupAuth      = "auth"
	groupMessaging = "messaging"
)

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	return filepath.Join(dir, "gotd", "gotd.cli.yaml")
}

// normalizeFlag maps deprecated/alternate flag names to their canonical form so
// older invocations keep working after the cobra migration.
func normalizeFlag(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	switch name {
	case "staging": // --staging -> --test
		name = "test"
	case "target": // --target -> --peer
		name = "peer"
	case "msg": // --msg -> --message
		name = "message"
	case "as": // --as -> --type
		name = "type"
	}
	return pflag.NormalizedName(name)
}

func newRootCmd() *cobra.Command {
	a := newApp()

	root := &cobra.Command{
		Use:   "tg",
		Short: "Telegram CLI for humans and agents",
		Long: `tg is a single static binary for driving a Telegram account from the
command line or an AI agent: send messages, upload and download files, and more.

Run "tg init" once to write a config, then use the subcommands. Global flags
below apply to every command.`,
		// We print errors ourselves in main; don't let cobra dump usage on every
		// runtime error (only on genuine flag/usage mistakes).
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.before(cmd)
		},
	}

	root.SetGlobalNormalizationFunc(normalizeFlag)

	pf := root.PersistentFlags()
	pf.StringVarP(&a.configPath, "config", "c", defaultConfigPath(), "config file to use")
	pf.BoolVar(&a.debugInvoker, "debug-invoker", false, "use pretty-printing debug invoker")
	pf.BoolVar(&a.testServer, "test", false, "connect to the telegram test server")

	root.AddGroup(
		&cobra.Group{ID: groupAuth, Title: "Authentication & setup:"},
		&cobra.Group{ID: groupMessaging, Title: "Messaging:"},
	)

	root.AddCommand(
		newInitCmd(a),
		a.newSendCmd(),
		a.newUploadCmd(),
	)
	root.AddCommand(newDocsCmd(root))

	return root
}
