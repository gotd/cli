package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-faster/errors"

	"github.com/gotd/td/tg"
)

// Reply markup types, the "type" of a replyMarkup.
const (
	markupInline     = "inline"      // buttons attached to the message
	markupKeyboard   = "keyboard"    // custom keyboard replacing the user's
	markupRemove     = "remove"      // hide the custom keyboard
	markupForceReply = "force_reply" // ask the user to reply to the message
)

// Button types handled specially by `tg click`. All types are named after the
// Telegram constructor, e.g. keyboardButtonSwitchInline -> "switch_inline".
const (
	buttonText     = "text" // plain reply-keyboard button: pressing sends its text
	buttonCallback = "callback"
	buttonGame     = "game"
	buttonURL      = "url"
	buttonURLAuth  = "url_auth"
	buttonCopy     = "copy"
)

// replyMarkup is a message's keyboard, as a bot sent it.
type replyMarkup struct {
	Type        string         `json:"type"`
	Rows        [][]buttonItem `json:"rows,omitempty"`
	Placeholder string         `json:"placeholder,omitempty"`
	SingleUse   bool           `json:"single_use,omitempty"`
	Persistent  bool           `json:"persistent,omitempty"`
}

// buttonItem is one keyboard button.
type buttonItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// Data is the callback data when it is printable text; binary data is
	// reported as DataBase64 instead.
	Data             string `json:"data,omitempty"`
	DataBase64       string `json:"data_base64,omitempty"`
	URL              string `json:"url,omitempty"`
	Query            string `json:"query,omitempty"`
	CopyText         string `json:"copy_text,omitempty"`
	RequiresPassword bool   `json:"requires_password,omitempty"`
}

// newReplyMarkup converts a message's reply markup; nil when there is none.
func newReplyMarkup(rm tg.ReplyMarkupClass) *replyMarkup {
	switch v := rm.(type) {
	case *tg.ReplyInlineMarkup:
		return &replyMarkup{Type: markupInline, Rows: buttonRows(v.Rows)}
	case *tg.ReplyKeyboardMarkup:
		return &replyMarkup{
			Type:        markupKeyboard,
			Rows:        buttonRows(v.Rows),
			Placeholder: v.Placeholder,
			SingleUse:   v.SingleUse,
			Persistent:  v.Persistent,
		}
	case *tg.ReplyKeyboardHide:
		return &replyMarkup{Type: markupRemove}
	case *tg.ReplyKeyboardForceReply:
		return &replyMarkup{Type: markupForceReply, Placeholder: v.Placeholder, SingleUse: v.SingleUse}
	default:
		return nil
	}
}

func buttonRows(rows []tg.KeyboardButtonRow) [][]buttonItem {
	out := make([][]buttonItem, 0, len(rows))
	for _, row := range rows {
		items := make([]buttonItem, 0, len(row.Buttons))
		for _, b := range row.Buttons {
			items = append(items, newButtonItem(b))
		}
		out = append(out, items)
	}
	return out
}

func newButtonItem(b tg.KeyboardButtonClass) buttonItem {
	item := buttonItem{Type: buttonType(b), Text: b.GetText()}
	switch v := b.(type) {
	case *tg.KeyboardButtonCallback:
		item.RequiresPassword = v.RequiresPassword
		if isPrintable(v.Data) {
			item.Data = string(v.Data)
		} else {
			item.DataBase64 = base64.StdEncoding.EncodeToString(v.Data)
		}
	case *tg.KeyboardButtonURL:
		item.URL = v.URL
	case *tg.KeyboardButtonURLAuth:
		item.URL = v.URL
	case *tg.KeyboardButtonWebView:
		item.URL = v.URL
	case *tg.KeyboardButtonSimpleWebView:
		item.URL = v.URL
	case *tg.KeyboardButtonSwitchInline:
		item.Query = v.Query
	case *tg.KeyboardButtonCopy:
		item.CopyText = v.CopyText
	}
	return item
}

// buttonType names a button after its constructor: keyboardButtonCallback ->
// "callback", keyboardButtonRequestGeoLocation -> "request_geo_location"; the
// plain keyboardButton is "text".
func buttonType(b tg.KeyboardButtonClass) string {
	name := strings.TrimPrefix(b.TypeName(), "keyboardButton")
	if name == "" {
		return buttonText
	}
	return snakeCase(name)
}

// snakeCase converts a CamelCase identifier to snake_case.
func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isPrintable reports whether callback data can be shown as text.
func isPrintable(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// label renders the markup compactly for text output, e.g.
// "[buttons: Yes | No / Back]".
func (m *replyMarkup) label() string {
	if m == nil {
		return ""
	}
	kind := "buttons"
	switch m.Type {
	case markupKeyboard:
		kind = "keyboard"
	case markupRemove:
		return "[keyboard removed]"
	case markupForceReply:
		return "[force reply]"
	}
	rows := make([]string, 0, len(m.Rows))
	for _, row := range m.Rows {
		labels := make([]string, 0, len(row))
		for _, b := range row {
			labels = append(labels, b.Text)
		}
		rows = append(rows, strings.Join(labels, " | "))
	}
	return "[" + kind + ": " + strings.Join(rows, " / ") + "]"
}

// keyboardButton is one button of a message's keyboard with its position.
type keyboardButton struct {
	Index    int // 1-based position in reading order
	Row, Col int // 1-based
	raw      tg.KeyboardButtonClass
	item     buttonItem
}

// keyboardButtons flattens a message's inline or reply keyboard in reading
// order. Markup without buttons yields nil.
func keyboardButtons(rm tg.ReplyMarkupClass) []keyboardButton {
	var rows []tg.KeyboardButtonRow
	switch v := rm.(type) {
	case *tg.ReplyInlineMarkup:
		rows = v.Rows
	case *tg.ReplyKeyboardMarkup:
		rows = v.Rows
	default:
		return nil
	}
	var out []keyboardButton
	for r, row := range rows {
		for c, b := range row.Buttons {
			out = append(out, keyboardButton{
				Index: len(out) + 1, Row: r + 1, Col: c + 1,
				raw: b, item: newButtonItem(b),
			})
		}
	}
	return out
}

// buttonSelector picks one button of a keyboard. Exactly one way of selecting
// is set: text, data, index, or row+col.
type buttonSelector struct {
	text  string // label: exact, case-insensitive, then unique substring
	data  string // exact callback data (text form, as printed in "data")
	index int    // 1-based position in reading order
	row   int    // 1-based row, with col
	col   int    // 1-based column, with row
}

// validate checks that exactly one selection method is set.
func (s buttonSelector) validate() error {
	n := 0
	for _, set := range []bool{s.text != "", s.data != "", s.index != 0, s.row != 0 || s.col != 0} {
		if set {
			n++
		}
	}
	switch {
	case n == 0:
		return usageError{errors.New("choose a button with --text, --data, --index or --row/--col")}
	case n > 1:
		return usageError{errors.New("use only one of --text, --data, --index or --row/--col")}
	case (s.row != 0) != (s.col != 0):
		return usageError{errors.New("--row and --col must be used together")}
	case s.index < 0 || s.row < 0 || s.col < 0:
		return usageError{errors.New("button positions are 1-based")}
	}
	return nil
}

// selectButton finds the button sel refers to.
func selectButton(buttons []keyboardButton, sel buttonSelector) (keyboardButton, error) {
	if len(buttons) == 0 {
		return keyboardButton{}, notFoundf("message has no keyboard buttons")
	}
	switch {
	case sel.index != 0:
		if sel.index > len(buttons) {
			return keyboardButton{}, notFoundf("no button #%d; %s", sel.index, describeButtons(buttons))
		}
		return buttons[sel.index-1], nil
	case sel.row != 0:
		for _, b := range buttons {
			if b.Row == sel.row && b.Col == sel.col {
				return b, nil
			}
		}
		return keyboardButton{}, notFoundf("no button at row %d, col %d; %s", sel.row, sel.col, describeButtons(buttons))
	case sel.data != "":
		for _, b := range buttons {
			if cb, ok := b.raw.(*tg.KeyboardButtonCallback); ok && string(cb.Data) == sel.data {
				return b, nil
			}
		}
		return keyboardButton{}, notFoundf("no callback button with data %q; %s", sel.data, describeButtons(buttons))
	default:
		return selectButtonByText(buttons, sel.text)
	}
}

// selectButtonByText matches a label exactly, then case-insensitively, then as
// a case-insensitive substring that must be unique (labels often carry emoji
// prefixes such as "📋 Menu").
func selectButtonByText(buttons []keyboardButton, text string) (keyboardButton, error) {
	for _, b := range buttons {
		if b.item.Text == text {
			return b, nil
		}
	}
	for _, b := range buttons {
		if strings.EqualFold(b.item.Text, text) {
			return b, nil
		}
	}
	needle := strings.ToLower(text)
	var matches []keyboardButton
	for _, b := range buttons {
		if strings.Contains(strings.ToLower(b.item.Text), needle) {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return keyboardButton{}, notFoundf("no button labeled %q; %s", text, describeButtons(buttons))
	default:
		return keyboardButton{}, notFoundf("%q matches %d buttons; be more specific: %s",
			text, len(matches), describeButtons(matches))
	}
}

// describeButtons lists buttons for error messages, e.g.
// `buttons: #1 "Yes" (1,1), #2 "No" (1,2)`.
func describeButtons(buttons []keyboardButton) string {
	parts := make([]string, 0, len(buttons))
	for _, b := range buttons {
		parts = append(parts, fmt.Sprintf("#%d %q (%d,%d)", b.Index, b.item.Text, b.Row, b.Col))
	}
	return "buttons: " + strings.Join(parts, ", ")
}
