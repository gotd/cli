package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-faster/errors"

	"github.com/gotd/td/tg"
)

func testInlineMarkup() *tg.ReplyInlineMarkup {
	return &tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{
		{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonCallback{Text: "📋 Menu", Data: []byte("menu")},
			&tg.KeyboardButtonCallback{Text: "Settings", Data: []byte{0xff, 0x00}},
		}},
		{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{Text: "Docs", URL: "https://example.com"},
			&tg.KeyboardButtonSwitchInline{Text: "Share", Query: "q"},
		}},
	}}
}

func TestNewReplyMarkupInline(t *testing.T) {
	m := newReplyMarkup(testInlineMarkup())
	if m == nil || m.Type != markupInline || len(m.Rows) != 2 {
		t.Fatalf("markup = %+v", m)
	}
	menu, settings, docs, share := m.Rows[0][0], m.Rows[0][1], m.Rows[1][0], m.Rows[1][1]
	if menu.Type != buttonCallback || menu.Data != "menu" || menu.DataBase64 != "" {
		t.Errorf("menu = %+v", menu)
	}
	if settings.Data != "" || settings.DataBase64 != "/wA=" {
		t.Errorf("binary data = %+v, want base64", settings)
	}
	if docs.Type != buttonURL || docs.URL != "https://example.com" {
		t.Errorf("docs = %+v", docs)
	}
	if share.Type != "switch_inline" || share.Query != "q" {
		t.Errorf("share = %+v", share)
	}
	if got := m.label(); got != "[buttons: 📋 Menu | Settings / Docs | Share]" {
		t.Errorf("label = %q", got)
	}
}

func TestNewReplyMarkupKinds(t *testing.T) {
	kb := newReplyMarkup(&tg.ReplyKeyboardMarkup{
		SingleUse:   true,
		Placeholder: "Pick one",
		Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButton{Text: "Yes"},
			&tg.KeyboardButtonRequestPhone{Text: "Share phone"},
		}}},
	})
	if kb.Type != markupKeyboard || !kb.SingleUse || kb.Placeholder != "Pick one" {
		t.Errorf("keyboard = %+v", kb)
	}
	if kb.Rows[0][0].Type != buttonText || kb.Rows[0][1].Type != "request_phone" {
		t.Errorf("button types = %q, %q", kb.Rows[0][0].Type, kb.Rows[0][1].Type)
	}
	if got := newReplyMarkup(&tg.ReplyKeyboardHide{}); got.Type != markupRemove {
		t.Errorf("hide = %+v", got)
	}
	if got := newReplyMarkup(&tg.ReplyKeyboardForceReply{}); got.Type != markupForceReply {
		t.Errorf("force reply = %+v", got)
	}
	if got := newReplyMarkup(nil); got != nil {
		t.Errorf("nil markup = %+v", got)
	}
}

func TestButtonTypeNames(t *testing.T) {
	for b, want := range map[tg.KeyboardButtonClass]string{
		&tg.KeyboardButton{}:                   "text",
		&tg.KeyboardButtonCallback{}:           "callback",
		&tg.KeyboardButtonURL{}:                "url",
		&tg.KeyboardButtonURLAuth{}:            "url_auth",
		&tg.KeyboardButtonWebView{}:            "web_view",
		&tg.KeyboardButtonSimpleWebView{}:      "simple_web_view",
		&tg.KeyboardButtonRequestGeoLocation{}: "request_geo_location",
		&tg.KeyboardButtonGame{}:               "game",
		&tg.KeyboardButtonCopy{}:               "copy",
	} {
		if got := buttonType(b); got != want {
			t.Errorf("buttonType(%T) = %q, want %q", b, got, want)
		}
	}
}

func TestSelectButton(t *testing.T) {
	buttons := keyboardButtons(testInlineMarkup())
	cases := []struct {
		name string
		sel  buttonSelector
		want string // label of the selected button; "" = expect not found
	}{
		{"exact", buttonSelector{text: "Settings"}, "Settings"},
		{"case-insensitive", buttonSelector{text: "settings"}, "Settings"},
		{"substring past emoji", buttonSelector{text: "menu"}, "📋 Menu"},
		{"no match", buttonSelector{text: "Logout"}, ""},
		{"data", buttonSelector{data: "menu"}, "📋 Menu"},
		{"data missing", buttonSelector{data: "nope"}, ""},
		{"index", buttonSelector{index: 3}, "Docs"},
		{"index out of range", buttonSelector{index: 9}, ""},
		{"row/col", buttonSelector{row: 2, col: 2}, "Share"},
		{"row/col missing", buttonSelector{row: 3, col: 1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := selectButton(buttons, c.sel)
			if c.want == "" {
				var nf notFoundError
				if !errors.As(err, &nf) {
					t.Fatalf("err = %v, want not-found", err)
				}
				if !strings.Contains(err.Error(), `#1 "📋 Menu" (1,1)`) {
					t.Errorf("error should list the buttons: %v", err)
				}
				return
			}
			if err != nil || got.item.Text != c.want {
				t.Fatalf("selected %q, %v; want %q", got.item.Text, err, c.want)
			}
		})
	}
}

func TestSelectButtonAmbiguous(t *testing.T) {
	buttons := keyboardButtons(&tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonCallback{Text: "Plan A", Data: []byte("a")},
		&tg.KeyboardButtonCallback{Text: "Plan B", Data: []byte("b")},
	}}}})
	_, err := selectButton(buttons, buttonSelector{text: "plan"})
	if err == nil || !strings.Contains(err.Error(), "matches 2 buttons") {
		t.Fatalf("err = %v, want ambiguity error", err)
	}
	if _, err := selectButton(nil, buttonSelector{text: "x"}); err == nil {
		t.Fatal("expected error for a message without buttons")
	}
}

func TestButtonSelectorValidate(t *testing.T) {
	valid := []buttonSelector{{text: "a"}, {data: "x"}, {index: 1}, {row: 1, col: 2}}
	for _, s := range valid {
		if err := s.validate(); err != nil {
			t.Errorf("%+v: %v", s, err)
		}
	}
	invalid := []buttonSelector{{}, {text: "a", index: 1}, {row: 1}, {col: 1}, {index: -1}}
	for _, s := range invalid {
		var u usageError
		if err := s.validate(); !errors.As(err, &u) {
			t.Errorf("%+v: err = %v, want usage error", s, err)
		}
	}
}

func TestBuildMessageItemDetails(t *testing.T) {
	msg := &tg.Message{
		ID:      10,
		PeerID:  &tg.PeerUser{UserID: 5},
		Message: "👋 Hi, see docs",
		Date:    100,
	}
	msg.SetEditDate(120)
	msg.SetViaBotID(77)
	msg.SetReplyMarkup(testInlineMarkup())
	// "👋" is two UTF-16 code units, so "Hi" starts at offset 3.
	msg.SetEntities([]tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 3, Length: 2},
		&tg.MessageEntityTextURL{Offset: 11, Length: 4, URL: "https://example.com"},
	})
	msg.SetMedia(&tg.MessageMediaDocument{Document: &tg.Document{
		MimeType: "audio/ogg",
		Size:     2048,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeAudio{Voice: true, Duration: 3},
		},
	}})

	item := buildMessageItem(msg, entitiesOf(nil, nil))
	if item.EditDate != 120 || item.ViaBotID != 77 {
		t.Errorf("edit/via = %d/%d", item.EditDate, item.ViaBotID)
	}
	if item.ReplyMarkup == nil || item.ReplyMarkup.Type != markupInline {
		t.Errorf("markup = %+v", item.ReplyMarkup)
	}
	if len(item.Entities) != 2 ||
		item.Entities[0].Type != "bold" || item.Entities[0].Text != "Hi" ||
		item.Entities[1].Type != "text_url" || item.Entities[1].Text != "docs" || item.Entities[1].URL != "https://example.com" {
		t.Errorf("entities = %+v", item.Entities)
	}
	if item.File == nil || item.File.Kind != "voice" || item.File.Size != 2048 || item.File.Duration != 3 {
		t.Errorf("file = %+v", item.File)
	}
}

func TestDocumentKinds(t *testing.T) {
	cases := map[string][]tg.DocumentAttributeClass{
		"file":       {&tg.DocumentAttributeFilename{FileName: "a.pdf"}},
		"video":      {&tg.DocumentAttributeVideo{Duration: 2, W: 640, H: 480}},
		"video_note": {&tg.DocumentAttributeVideo{RoundMessage: true}},
		"audio":      {&tg.DocumentAttributeAudio{Duration: 60}},
		"animation":  {&tg.DocumentAttributeVideo{}, &tg.DocumentAttributeAnimated{}},
		"sticker":    {&tg.DocumentAttributeImageSize{W: 512, H: 512}, &tg.DocumentAttributeSticker{}},
	}
	for want, attrs := range cases {
		if got := documentInfo(&tg.Document{Attributes: attrs}).Kind; got != want {
			t.Errorf("kind = %q, want %q", got, want)
		}
	}
	if name := documentInfo(&tg.Document{Attributes: cases["file"]}).Name; name != "a.pdf" {
		t.Errorf("name = %q", name)
	}
}

// TestMessageItemJSONCompat pins the JSON of a plain message to the shape
// consumers parsed before the bot-testing fields were added.
func TestMessageItemJSONCompat(t *testing.T) {
	msg := &tg.Message{ID: 3, PeerID: &tg.PeerUser{UserID: 5}, Message: "hi", Date: 9, Out: true}
	got, err := json.Marshal(buildMessageItem(msg, entitiesOf(nil, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":3,"date":9,"out":true,"text":"hi"}`; string(got) != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}
