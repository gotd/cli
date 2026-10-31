package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/atomicfile"
)

// webAppDefaultPlatform is the client the session is requested as.
const webAppDefaultPlatform = "tdesktop"

// webAppResult is the result of `tg webapp`. It never carries the signed
// launch data unless --reveal asks for it: that data is a credential for the
// Mini App's user until the app's own expiry.
type webAppResult struct {
	Peer peerRef `json:"peer"`
	// Source is what opened the app: "menu_button", "button", "url" or "main".
	Source string `json:"source"`
	// URL is the page, without its signed fragment.
	URL      string `json:"url"`
	Platform string `json:"platform,omitempty"`
	Version  string `json:"version,omitempty"`
	// Signed is true when Telegram returned launch data (tgWebAppData).
	Signed     bool   `json:"signed"`
	UserID     int64  `json:"user_id,omitempty"`
	AuthDate   int64  `json:"auth_date,omitempty"`
	QueryID    string `json:"query_id,omitempty"`
	StartParam string `json:"start_param,omitempty"`
	// OutFile is where --out wrote the signed URL and launch data.
	OutFile string `json:"out_file,omitempty"`
	// SignedURL and InitData are set only with --reveal.
	SignedURL string `json:"signed_url,omitempty"`
	InitData  string `json:"init_data,omitempty"`
}

// MarshalText renders the session without its credential.
func (r webAppResult) MarshalText(w io.Writer) error {
	lines := []string{fmt.Sprintf("%s Mini App (%s): %s", r.Peer.label(), r.Source, r.URL)}
	if r.Signed {
		line := fmt.Sprintf("signed launch data: user %d", r.UserID)
		if r.AuthDate != 0 {
			line += ", issued " + time.Unix(r.AuthDate, 0).UTC().Format(time.RFC3339)
		}
		lines = append(lines, line)
	} else {
		lines = append(lines, "no signed launch data returned")
	}
	if r.QueryID != "" {
		lines = append(lines, "query id: "+r.QueryID)
	}
	if r.OutFile != "" {
		lines = append(lines, "written to "+r.OutFile)
	}
	if r.SignedURL != "" {
		lines = append(lines, "signed url: "+r.SignedURL)
	}
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

// webAppLaunch is the launch data parsed from a web view URL.
type webAppLaunch struct {
	Page       string // the URL without its fragment
	Version    string
	Platform   string
	InitData   string // the raw tgWebAppData value, a credential
	UserID     int64
	AuthDate   int64
	QueryID    string
	StartParam string
}

// parseWebAppURL splits a web view URL into its page and launch data. Telegram
// puts the launch data in the fragment, so it never reaches the app's server
// in the request line.
func parseWebAppURL(raw string) (webAppLaunch, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// Parse errors quote the input, which here holds the credential.
		return webAppLaunch{}, errors.New("telegram returned a web view URL that does not parse")
	}
	fragment := u.EscapedFragment() // Fragment is already decoded once, and ParseQuery decodes it
	u.Fragment, u.RawFragment = "", ""
	launch := webAppLaunch{Page: u.String()}
	values, err := url.ParseQuery(fragment)
	if err != nil {
		return webAppLaunch{}, errors.New("telegram returned a web view fragment that does not parse")
	}
	launch.Version = values.Get("tgWebAppVersion")
	launch.Platform = values.Get("tgWebAppPlatform")
	launch.InitData = values.Get("tgWebAppData")
	if launch.InitData == "" {
		return launch, nil
	}
	data, err := url.ParseQuery(launch.InitData)
	if err != nil {
		return webAppLaunch{}, errors.New("telegram returned launch data that does not parse")
	}
	launch.AuthDate, _ = strconv.ParseInt(data.Get("auth_date"), 10, 64)
	launch.QueryID = data.Get("query_id")
	launch.StartParam = data.Get("start_param")
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(data.Get("user")), &user); err == nil {
		launch.UserID = user.ID
	}
	return launch, nil
}

// webAppSecret is what --out writes: everything a browser or test needs.
type webAppSecret struct {
	URL      string `json:"url"`
	InitData string `json:"init_data"`
	Page     string `json:"page"`
}

// writeWebAppSecret writes the signed session to path, owner-only.
func writeWebAppSecret(path, signedURL string, launch webAppLaunch) error {
	data, err := json.MarshalIndent(webAppSecret{URL: signedURL, InitData: launch.InitData, Page: launch.Page}, "", "  ")
	if err != nil {
		return errors.Wrap(err, "encode session")
	}
	if err := atomicfile.Create(path, append(data, '\n'), 0o600); err != nil {
		return errors.Wrap(err, "write session")
	}
	return nil
}

// webAppButtonURL returns the Mini App URL of a keyboard button, and whether
// it is the kind a client opens with messages.requestSimpleWebView.
func webAppButtonURL(b tg.KeyboardButtonClass) (target string, simple, ok bool) {
	switch v := b.(type) {
	case *tg.KeyboardButtonWebView:
		return v.URL, false, true
	case *tg.KeyboardButtonSimpleWebView:
		return v.URL, true, true
	}
	return "", false, false
}

func (a *app) newWebAppCmd() *cobra.Command {
	var (
		pageURL    string
		text       string
		main       bool
		startParam string
		platform   string
		out        string
		reveal     bool
	)
	cmd := &cobra.Command{
		Use:     "webapp <bot>",
		Short:   "Open a bot's Mini App and get its signed launch data",
		GroupID: groupMessaging,
		Long: `Ask Telegram to open a bot's Mini App, the way a client does, and report the
session: the page and the identity Telegram signed for this account. A real
signed session lets a test drive the app's API or load it in a browser with no
Telegram client:

  Authorization: tma <init_data>     for the app's API
  the signed URL in a browser        window.Telegram.WebApp reads its fragment

By default this opens the bot's menu button. --text opens a web app button from
the bot's newest keyboard, --url a URL of your choice, --main the bot's main
Mini App (with a --start-param if it takes one).

The launch data is a credential: anyone holding it can act as this account in
that Mini App until the app's own expiry. It is printed only with --reveal, and
--out writes it to an owner-only file. Opening an app reveals this account's id,
name and language to the bot's server, as any client does.`,
		Example: `  tg webapp @mybot
  tg webapp @mybot --out session.json && jq -r .url session.json
  tg webapp @mybot --text "Open store" -o json
  tg webapp @mybot --main --start-param promo`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if (pageURL != "" && text != "") || (main && (pageURL != "" || text != "")) {
				return usageError{errors.New("use only one of --url, --text and --main")}
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				bot, err := resolveBot(ctx, m, args[0])
				if err != nil {
					return err
				}
				inputUser, peer := bot.InputUser(), bot.InputPeer()

				source := "url"
				target := pageURL
				simple := false
				switch {
				case main:
					source = "main"
				case text != "":
					source = "button"
					msg, err := keyboardMessage(ctx, api, peer, 0)
					if err != nil {
						return err
					}
					btn, err := selectButtonByText(keyboardButtons(msg.ReplyMarkup), text)
					if err != nil {
						return err
					}
					var ok bool
					if target, simple, ok = webAppButtonURL(btn.raw); !ok {
						return usageError{errors.Errorf("button %q does not open a Mini App", text)}
					}
				case target == "":
					source = "menu_button"
					full, err := bot.FullRaw(ctx)
					if err != nil {
						return errors.Wrap(err, "users.getFullUser")
					}
					info, ok := full.GetBotInfo()
					if !ok {
						return notFoundf("%s has no bot info", args[0])
					}
					menu, ok := info.MenuButton.(*tg.BotMenuButton)
					if !ok {
						return notFoundf("%s has no Mini App menu button", args[0])
					}
					target = menu.URL
				}

				var result *tg.WebViewResultURL
				switch {
				case main:
					result, err = api.MessagesRequestMainWebView(ctx, &tg.MessagesRequestMainWebViewRequest{
						Peer: peer, Bot: inputUser, StartParam: startParam, Platform: platform,
					})
					err = errors.Wrap(err, "messages.requestMainWebView")
				case simple:
					result, err = api.MessagesRequestSimpleWebView(ctx, &tg.MessagesRequestSimpleWebViewRequest{
						Bot: inputUser, URL: target, Platform: platform,
					})
					err = errors.Wrap(err, "messages.requestSimpleWebView")
				default:
					result, err = api.MessagesRequestWebView(ctx, &tg.MessagesRequestWebViewRequest{
						FromBotMenu: source == "menu_button",
						Peer:        peer, Bot: inputUser, URL: target, StartParam: startParam, Platform: platform,
					})
					err = errors.Wrap(err, "messages.requestWebView")
				}
				if err != nil {
					return err
				}

				launch, err := parseWebAppURL(result.URL)
				if err != nil {
					return err
				}
				res := webAppResult{
					Peer: describeManagedPeer(bot), Source: source, URL: launch.Page,
					Platform: launch.Platform, Version: launch.Version, Signed: launch.InitData != "",
					UserID: launch.UserID, AuthDate: launch.AuthDate, QueryID: launch.QueryID, StartParam: launch.StartParam,
				}
				if out != "" {
					if err := writeWebAppSecret(out, result.URL, launch); err != nil {
						return err
					}
					res.OutFile = out
				}
				if reveal {
					res.SignedURL, res.InitData = result.URL, launch.InitData
				}
				return a.printer.Emit(res)
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&pageURL, "url", "", "open this URL as the bot's Mini App")
	fs.StringVar(&text, "text", "", "open the web app button with this label")
	fs.BoolVar(&main, "main", false, "open the bot's main Mini App")
	fs.StringVar(&startParam, "start-param", "", "start parameter passed to the app")
	fs.StringVar(&platform, "platform", webAppDefaultPlatform, "client platform to report (tdesktop, android, ios, web)")
	fs.StringVar(&out, "out", "", "write the signed URL and launch data to this owner-only file")
	fs.BoolVar(&reveal, "reveal", false, "print the signed URL and launch data (a credential)")

	return cmd
}
