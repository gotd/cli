package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

// botCommand is one command from a bot's command menu.
type botCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// menuButton is the button next to a bot chat's input field.
type menuButton struct {
	// Type is "commands" (opens the command list), "web_app" or "default".
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

// botInfoResult is the result of `tg bot-info`.
type botInfoResult struct {
	Peer peerRef `json:"peer"`
	// About is the short description shown on the bot's profile.
	About string `json:"about,omitempty"`
	// Description is the "What can this bot do?" text of an empty chat.
	Description      string       `json:"description,omitempty"`
	Commands         []botCommand `json:"commands"`
	MenuButton       *menuButton  `json:"menu_button,omitempty"`
	PrivacyPolicyURL string       `json:"privacy_policy_url,omitempty"`
	// InlinePlaceholder is set when the bot supports inline mode.
	InlinePlaceholder     string `json:"inline_placeholder,omitempty"`
	CanJoinGroups         bool   `json:"can_join_groups"`
	ReadsAllGroupMessages bool   `json:"reads_all_group_messages"`
	ActiveUsers           int    `json:"active_users,omitempty"`
}

// MarshalText renders the bot's profile and command menu.
func (r botInfoResult) MarshalText(w io.Writer) error {
	lines := []string{fmt.Sprintf("%s id=%d", r.Peer.label(), r.Peer.ID)}
	if r.About != "" {
		lines = append(lines, "about: "+r.About)
	}
	if r.Description != "" {
		lines = append(lines, "description: "+strings.ReplaceAll(r.Description, "\n", " "))
	}
	if r.MenuButton != nil {
		lines = append(lines, "menu button: "+strings.TrimSpace(r.MenuButton.Type+" "+r.MenuButton.Text+" "+r.MenuButton.URL))
	}
	if r.InlinePlaceholder != "" {
		lines = append(lines, fmt.Sprintf("inline mode: %q", r.InlinePlaceholder))
	}
	lines = append(lines, fmt.Sprintf("commands (%d):", len(r.Commands)))
	for _, c := range r.Commands {
		lines = append(lines, fmt.Sprintf("  /%s - %s", c.Command, c.Description))
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

// newBotInfoResult builds the result from a bot user and its full profile.
func newBotInfoResult(ref peerRef, u *tg.User, full *tg.UserFull) botInfoResult {
	res := botInfoResult{
		Peer:                  ref,
		About:                 full.About,
		Commands:              []botCommand{},
		InlinePlaceholder:     u.BotInlinePlaceholder,
		CanJoinGroups:         !u.BotNochats,
		ReadsAllGroupMessages: u.BotChatHistory,
		ActiveUsers:           u.BotActiveUsers,
	}
	info, ok := full.GetBotInfo()
	if !ok {
		return res
	}
	res.Description = info.Description
	res.PrivacyPolicyURL = info.PrivacyPolicyURL
	for _, c := range info.Commands {
		res.Commands = append(res.Commands, botCommand{Command: c.Command, Description: c.Description})
	}
	switch b := info.MenuButton.(type) {
	case *tg.BotMenuButtonCommands:
		res.MenuButton = &menuButton{Type: "commands"}
	case *tg.BotMenuButton:
		res.MenuButton = &menuButton{Type: "web_app", Text: b.Text, URL: b.URL}
	case *tg.BotMenuButtonDefault:
		res.MenuButton = &menuButton{Type: "default"}
	}
	return res
}

// resolveBot resolves arg and checks that it is a bot.
func resolveBot(ctx context.Context, m *peerManager, arg string) (peers.User, error) {
	p, err := resolveManagedPeer(ctx, m, arg)
	if err != nil {
		return peers.User{}, err
	}
	u, ok := p.(peers.User)
	if !ok || !u.Raw().Bot {
		return peers.User{}, usageError{errors.Errorf("%s is not a bot", arg)}
	}
	return u, nil
}

func (a *app) newBotInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "bot-info <bot>",
		Short:   "Show a bot's description, commands and capabilities",
		GroupID: groupMessaging,
		Long: `Show what a bot advertises: its profile and empty-chat descriptions, the
command menu (/commands with descriptions), the menu button, inline mode and
group settings. Handy as the first step of testing a bot: every listed command
can then be exercised with ` + "`tg send --peer <bot> /<command> --wait 30s`.",
		Example: `  tg bot-info @BotFather
  tg bot-info @mybot -o json | jq -r '.data.commands[].command'`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				u, err := resolveBot(ctx, m, args[0])
				if err != nil {
					return err
				}
				full, err := u.FullRaw(ctx)
				if err != nil {
					return errors.Wrap(err, "users.getFullUser")
				}
				return a.printer.Emit(newBotInfoResult(describeManagedPeer(u), u.Raw(), full))
			})
		},
	}
}

// parseBotLink parses a bot deep link such as https://t.me/mybot?start=abc or
// tg://resolve?domain=mybot&start=abc into "@mybot" and the start parameter.
func parseBotLink(s string) (bot, param string, ok bool) {
	raw := s
	if !strings.Contains(raw, "://") {
		if !strings.HasPrefix(raw, "t.me/") && !strings.HasPrefix(raw, "telegram.me/") {
			return "", "", false
		}
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	switch {
	case u.Scheme == "tg" && u.Host == "resolve":
		bot = u.Query().Get("domain")
	case (u.Scheme == "https" || u.Scheme == "http") && (u.Host == "t.me" || u.Host == "telegram.me"):
		bot = strings.Trim(u.Path, "/")
	default:
		return "", "", false
	}
	if bot == "" || strings.Contains(bot, "/") {
		return "", "", false
	}
	return "@" + bot, u.Query().Get("start"), true
}

// startArgs splits `start-bot` arguments into the bot and start parameter.
func startArgs(args []string) (bot, param string, err error) {
	bot = args[0]
	if b, p, ok := parseBotLink(bot); ok {
		bot, param = b, p
	}
	if len(args) == 2 {
		if param != "" {
			return "", "", usageError{errors.New("start parameter given both in the link and as an argument")}
		}
		param = args[1]
	}
	return bot, param, nil
}

func (a *app) newStartBotCmd() *cobra.Command {
	var wait waitOptions

	cmd := &cobra.Command{
		Use:     "start-bot <bot> [param]",
		Short:   "Start a bot, optionally with a deep-link parameter",
		GroupID: groupMessaging,
		Long: `Start a bot the way a t.me/<bot>?start=<param> deep link does: the bot
receives "/start <param>" as a deep-link start, so referral, login and linking
flows can be tested. Without a parameter a plain "/start" is sent. The bot may
be given as the deep link itself.

With --wait the bot's replies (and message edits) are collected and printed.`,
		Example: `  tg start-bot @mybot --wait 20s
  tg start-bot @mybot ref_42 --wait 20s -o json
  tg start-bot "https://t.me/mybot?start=ref_42" --wait 20s`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			botArg, param, err := startArgs(args)
			if err != nil {
				return err
			}
			collector := newReplyCollector()
			return a.run(cmd.Context(), wait.runParams(collector), func(ctx context.Context, api *tg.Client) error {
				sender, m, err := a.sender(api)
				if err != nil {
					return err
				}
				bot, err := resolveBot(ctx, m, botArg)
				if err != nil {
					return err
				}
				collector.arm(bot.ID())

				var id int
				if param == "" {
					id, err = unpack.MessageID(sender.To(bot.InputPeer()).Text(ctx, "/start"))
				} else {
					var rid int64
					if rid, err = randomID(); err != nil {
						return err
					}
					id, err = unpack.MessageID(api.MessagesStartBot(ctx, &tg.MessagesStartBotRequest{
						Bot:        bot.InputUser(),
						Peer:       bot.InputPeer(),
						RandomID:   rid,
						StartParam: param,
					}))
				}
				if err != nil {
					return errors.Wrap(err, "start bot")
				}

				res := sentResult{Peer: botArg, MessageID: id}
				if res.conversation, err = awaitResponse(ctx, collector, wait); err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}
	wait.register(cmd.Flags())
	return cmd
}
