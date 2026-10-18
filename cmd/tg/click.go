package main

import (
	"context"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/unpack"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// keyboardSearchDepth is how many recent messages `tg click` scans for a
// keyboard when no message id is given.
const keyboardSearchDepth = 20

// callbackAnswer is a bot's answer to a pressed callback button.
type callbackAnswer struct {
	// Answered is false when the bot did not answer the callback query in
	// time (BOT_RESPONSE_TIMEOUT); it may still have acted on the press.
	Answered bool `json:"answered"`
	// Message is the notification text; Alert means it is shown as a modal
	// alert instead of a toast.
	Message string `json:"message,omitempty"`
	Alert   bool   `json:"alert,omitempty"`
	URL     string `json:"url,omitempty"`
}

// clickResult is the result of `tg click`.
type clickResult struct {
	Peer      string     `json:"peer,omitempty"`
	MessageID int        `json:"message_id"` // the message carrying the keyboard
	Row       int        `json:"row"`
	Col       int        `json:"col"`
	Button    buttonItem `json:"button"`
	// Answer is the callback answer (callback and game buttons).
	Answer *callbackAnswer `json:"answer,omitempty"`
	// SentMessageID is the message sent by a reply-keyboard button.
	SentMessageID int `json:"sent_message_id,omitempty"`
	*conversation
}

// MarshalText renders what was pressed and what came back.
func (r clickResult) MarshalText(w io.Writer) error {
	line := fmt.Sprintf("pressed %q (%s, row %d, col %d) on message #%d",
		r.Button.Text, r.Button.Type, r.Row, r.Col, r.MessageID)
	switch {
	case r.SentMessageID != 0:
		line += fmt.Sprintf(": sent message #%d", r.SentMessageID)
	case r.Button.URL != "":
		line += ": " + r.Button.URL
	case r.Button.CopyText != "":
		line += fmt.Sprintf(": copy %q", r.Button.CopyText)
	}
	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}
	if a := r.Answer; a != nil {
		var ans string
		switch {
		case !a.Answered:
			ans = "answer: none (bot did not answer in time)"
		case a.Alert:
			ans = fmt.Sprintf("answer (alert): %q", a.Message)
		case a.Message != "":
			ans = fmt.Sprintf("answer: %q", a.Message)
		default:
			ans = "answer: (empty)"
		}
		if a.URL != "" {
			ans += " url=" + a.URL
		}
		if _, err := fmt.Fprintln(w, ans); err != nil {
			return err
		}
	}
	return r.conversation.marshalText(w)
}

// keyboardMessage returns the message whose keyboard to press: message id, or
// with id 0 the newest incoming message from peer that has buttons.
func keyboardMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, id int) (*tg.Message, error) {
	if id != 0 {
		msg, err := getMessage(ctx, api, peer, id)
		if err != nil {
			return nil, err
		}
		if len(keyboardButtons(msg.ReplyMarkup)) == 0 {
			return nil, notFoundf("message #%d has no keyboard buttons", id)
		}
		return msg, nil
	}

	res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, Limit: keyboardSearchDepth})
	if err != nil {
		return nil, errors.Wrap(err, "messages.getHistory")
	}
	msgs, _, err := messagesFrom(res)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs { // newest first
		if msg, ok := m.(*tg.Message); ok && !msg.Out && len(keyboardButtons(msg.ReplyMarkup)) > 0 {
			return msg, nil
		}
	}
	return nil, notFoundf("none of the last %d messages from this peer has keyboard buttons", keyboardSearchDepth)
}

// pressButton performs a button's action the way a Telegram client does.
// to sends messages to the chat (for reply-keyboard buttons).
func pressButton(
	ctx context.Context,
	api *tg.Client,
	to *message.RequestBuilder,
	peer tg.InputPeerClass,
	msgID int,
	b keyboardButton,
) (clickResult, error) {
	res := clickResult{MessageID: msgID, Row: b.Row, Col: b.Col, Button: b.item}
	switch v := b.raw.(type) {
	case *tg.KeyboardButtonCallback:
		if v.RequiresPassword {
			return res, errors.New("this button requires the account's 2FA password; tg does not send it")
		}
		req := &tg.MessagesGetBotCallbackAnswerRequest{Peer: peer, MsgID: msgID}
		req.SetData(v.Data)
		ans, err := botCallbackAnswer(ctx, api, req)
		res.Answer = ans
		return res, err
	case *tg.KeyboardButtonGame:
		req := &tg.MessagesGetBotCallbackAnswerRequest{Peer: peer, MsgID: msgID}
		req.SetGame(true)
		ans, err := botCallbackAnswer(ctx, api, req)
		res.Answer = ans
		return res, err
	case *tg.KeyboardButton:
		// A reply-keyboard button sends its label. Outside private chats it is
		// sent as a reply to the keyboard, so bots in privacy mode receive it.
		mb := &to.Builder
		if _, private := peer.(*tg.InputPeerUser); !private {
			mb = mb.Reply(msgID)
		}
		id, err := unpack.MessageID(mb.Text(ctx, v.Text))
		if err != nil {
			return res, errors.Wrap(err, "send button text")
		}
		res.SentMessageID = id
		return res, nil
	case *tg.KeyboardButtonURL, *tg.KeyboardButtonURLAuth, *tg.KeyboardButtonWebView,
		*tg.KeyboardButtonSimpleWebView, *tg.KeyboardButtonCopy:
		// These act only in the client: nothing reaches the bot. The result
		// carries the URL or text the client would open or copy.
		return res, nil
	default:
		return res, errors.Errorf("%s buttons cannot be pressed from tg", b.item.Type)
	}
}

// botCallbackAnswer sends a callback query and returns the bot's answer. A bot
// that does not answer in time is not an error: it may still have acted.
func botCallbackAnswer(ctx context.Context, api *tg.Client, req *tg.MessagesGetBotCallbackAnswerRequest) (*callbackAnswer, error) {
	ans, err := api.MessagesGetBotCallbackAnswer(ctx, req)
	if tgerr.Is(err, "BOT_RESPONSE_TIMEOUT") {
		return &callbackAnswer{Answered: false}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "messages.getBotCallbackAnswer")
	}
	return &callbackAnswer{Answered: true, Message: ans.Message, Alert: ans.Alert, URL: ans.URL}, nil
}

func (a *app) newClickCmd() *cobra.Command {
	var (
		sel  buttonSelector
		wait waitOptions
	)

	cmd := &cobra.Command{
		Use:     "click <peer> [message-id]",
		Aliases: []string{"press"},
		Short:   "Press a bot's inline or reply-keyboard button",
		GroupID: groupMessaging,
		Long: `Press a button of a bot's keyboard, as a Telegram client would:

  callback   sends the callback query and prints the bot's answer (toast/alert)
  text       (reply keyboard) sends the button's label as a message
  url, copy  nothing is sent; the URL or text is printed

Choose the button with --text (exact label, then case-insensitive, then a
unique substring), --data (callback data), --index (1-based, reading order) or
--row/--col. Without a message id, the newest message from the peer that has
buttons is used. ` + "`tg history <peer> -o json`" + ` shows each message's
"reply_markup".

With --wait the bot's response is collected: new messages and edits (bots often
edit the message whose button was pressed).`,
		Example: `  tg click @mybot --text "Settings" --wait 15s
  tg click @mybot 4242 --data "page:2" --wait 15s -o json
  tg click @mybot --row 2 --col 1`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sel.validate(); err != nil {
				return err
			}
			var msgID int
			if len(args) == 2 {
				id, err := parseMessageID(args[1])
				if err != nil {
					return err
				}
				msgID = id
			}

			collector := newReplyCollector()
			return a.run(cmd.Context(), wait.runParams(collector), func(ctx context.Context, api *tg.Client) error {
				sender, m, err := a.sender(api)
				if err != nil {
					return err
				}
				peer, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}
				msg, err := keyboardMessage(ctx, api, peer, msgID)
				if err != nil {
					return err
				}
				btn, err := selectButton(keyboardButtons(msg.ReplyMarkup), sel)
				if err != nil {
					return err
				}

				collector.arm(inputPeerID(peer))
				res, err := pressButton(ctx, api, sender.To(peer), peer, msg.ID, btn)
				if err != nil {
					return err
				}
				res.Peer = args[0]
				if res.conversation, err = awaitResponse(ctx, collector, wait); err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&sel.text, "text", "", "press the button with this label")
	fs.StringVar(&sel.data, "data", "", "press the callback button with this data")
	fs.IntVar(&sel.index, "index", 0, "press the n-th button (1-based, reading order)")
	fs.IntVar(&sel.row, "row", 0, "press the button in this row (1-based, with --col)")
	fs.IntVar(&sel.col, "col", 0, "press the button in this column (1-based, with --row)")
	wait.register(fs)

	return cmd
}
