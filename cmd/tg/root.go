package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/gotd/cli/internal/output"
)

// Command groups shown in `tg --help`.
const (
	groupAuth      = "auth"
	groupMessaging = "messaging"
	groupChats     = "chats"
)

// cmdList is the common "list" subcommand verb.
const cmdList = "list"

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
	pf.StringVarP(&a.outputFormat, "output", "o", string(output.Text), "output format: text or json")
	pf.StringVar(&a.proxyURL, "proxy", os.Getenv("TG_PROXY"),
		"proxy URL: socks5:// or tg://proxy?... (overrides config)")
	pf.BoolVar(&a.debugInvoker, "debug-invoker", false, "use pretty-printing debug invoker")
	pf.BoolVar(&a.testServer, "test", false, "connect to the telegram test server")
	_ = root.RegisterFlagCompletionFunc("output",
		cobra.FixedCompletions(output.Formats(), cobra.ShellCompDirectiveNoFileComp))

	root.AddGroup(
		&cobra.Group{ID: groupAuth, Title: "Authentication & setup:"},
		&cobra.Group{ID: groupMessaging, Title: "Messaging:"},
		&cobra.Group{ID: groupChats, Title: "Chats & contacts:"},
		&cobra.Group{ID: groupProfile, Title: "Profile & folders:"},
	)

	root.AddCommand(
		newInitCmd(a),
		a.newLoginCmd(),
		a.newWhoamiCmd(),
		a.newChatsCmd(),
		a.newHistoryCmd(),
		a.newSendCmd(),
		a.newReplyCmd(),
		a.newEditCmd(),
		a.newDeleteCmd(),
		a.newDeleteHistoryCmd(),
		a.newForwardCmd(),
		a.newPinCmd(),
		a.newUnpinCmd(),
		a.newUnpinAllCmd(),
		a.newPinnedCmd(),
		a.newSearchCmd(),
		a.newReactCmd(),
		a.newUnreactCmd(),
		a.newReactionsCmd(),
		a.newDraftCmd(),
		a.newDraftsCmd(),
		a.newLinkCmd(),
		a.newContextCmd(),
		a.newScheduleCmd(),
		a.newPollCmd(),
		a.newReadCmd(),
		a.newUploadCmd(),
		a.newAlbumCmd(),
		a.newStickersCmd(),
		a.newDownloadCmd(),
		a.newMuteCmd(),
		a.newUnmuteCmd(),
		a.newArchiveCmd(),
		a.newUnarchiveCmd(),
		a.newChatCmd(),
		a.newResolveCmd(),
		a.newSearchPublicCmd(),
		a.newSubscribeCmd(),
		a.newContactsCmd(),
		a.newCreateGroupCmd(),
		a.newCreateChannelCmd(),
		a.newInviteCmd(),
		a.newLeaveCmd(),
		a.newParticipantsCmd(),
		a.newAdminsCmd(),
		a.newBannedCmd(),
		a.newPromoteCmd(),
		a.newDemoteCmd(),
		a.newBanCmd(),
		a.newUnbanCmd(),
		a.newSlowModeCmd(),
		a.newSetTitleCmd(),
		a.newSetAboutCmd(),
		a.newSetPhotoCmd(),
		a.newInviteLinkCmd(),
		a.newJoinLinkCmd(),
		a.newTopicsCmd(),
		a.newRecentActionsCmd(),
		a.newProfileCmd(),
		a.newFoldersCmd(),
	)
	root.AddCommand(newDocsCmd(root))

	return root
}
