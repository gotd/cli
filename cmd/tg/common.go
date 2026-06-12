package main

import (
	"time"

	"github.com/spf13/pflag"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"
)

// messageOptions holds flags shared by the send and upload commands.
type messageOptions struct {
	html      bool
	silent    bool
	noWebpage bool
	schedule  time.Duration
}

// register binds the shared message flags onto the given flag set.
func (o *messageOptions) register(fs *pflag.FlagSet) {
	fs.BoolVar(&o.html, "html", false, "use HTML styling")
	fs.BoolVar(&o.silent, "silent", false,
		"send this message silently (no notifications for the receivers)")
	fs.BoolVar(&o.noWebpage, "nowebpage", false,
		"disable generation of the webpage preview")
	fs.DurationVar(&o.schedule, "schedule", 0,
		"schedule the message to be sent after this delay")
}

// apply mutates the request builder according to the flags and returns the
// builder together with the styled text options for the message body.
func (o *messageOptions) apply(
	r *message.RequestBuilder,
	msg string,
) (*message.Builder, []styling.StyledTextOption) {
	b := &r.Builder

	if o.silent {
		b = b.Silent()
	}
	if o.noWebpage {
		b = b.NoWebpage()
	}
	if o.schedule > 0 {
		b = b.Schedule(time.Now().Add(o.schedule))
	}

	option := styling.Plain(msg)
	if o.html {
		option = html.String(nil, msg)
	}

	return b, []styling.StyledTextOption{option}
}
