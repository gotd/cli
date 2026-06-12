package main

import (
	"github.com/spf13/cobra"
)

// registerPeerCompletion wires dynamic shell completion for a peer-taking flag.
//
// TODO(Phase 0): read resolved dialogs/usernames from the local peer cache so
// completion reflects the user's actual chats without a network round-trip. For
// now it offers the always-valid self aliases.
func registerPeerCompletion(cmd *cobra.Command, flag string) {
	_ = cmd.RegisterFlagCompletionFunc(flag,
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{
					"me\tSaved Messages (yourself)",
					"self\tSaved Messages (yourself)",
				},
				cobra.ShellCompDirectiveNoFileComp
		},
	)
}

// peerArgCompletion completes a positional peer argument (the first arg).
func peerArgCompletion(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return []string{
			"me\tSaved Messages (yourself)",
			"self\tSaved Messages (yourself)",
		},
		cobra.ShellCompDirectiveNoFileComp
}

// noFileComp disables file completion for positional args.
func noFileComp(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// registerEnumCompletion offers the allowed values of an enum flag.
func registerEnumCompletion(cmd *cobra.Command, flag string, allowed []string) {
	_ = cmd.RegisterFlagCompletionFunc(flag,
		cobra.FixedCompletions(allowed, cobra.ShellCompDirectiveNoFileComp),
	)
}
