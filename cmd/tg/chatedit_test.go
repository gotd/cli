package main

import "testing"

func TestInviteHash(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://t.me/+AbCdEf", "AbCdEf"},
		{"https://t.me/joinchat/XyZ", "XyZ"},
		{"t.me/+Hash123", "Hash123"},
		{"+Hash123", "Hash123"},
		{"PlainHash", "PlainHash"},
	} {
		if got := inviteHash(tc.in); got != tc.want {
			t.Errorf("inviteHash(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
