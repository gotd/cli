package main

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestTermAuthPresetValues(t *testing.T) {
	ta := termAuth{phone: "+100", password: "secret"}
	ctx := context.Background()

	if got, err := ta.Phone(ctx); err != nil || got != "+100" {
		t.Errorf("Phone() = %q, %v", got, err)
	}
	if got, err := ta.Password(ctx); err != nil || got != "secret" {
		t.Errorf("Password() = %q, %v", got, err)
	}
}

func TestTermAuthPrompts(t *testing.T) {
	var out bytes.Buffer
	ta := termAuth{
		in:  bufio.NewReader(strings.NewReader("+199\n123456\n")),
		out: &out,
	}
	ctx := context.Background()

	phone, err := ta.Phone(ctx)
	if err != nil || phone != "+199" {
		t.Fatalf("Phone() = %q, %v", phone, err)
	}
	code, err := ta.Code(ctx, nil)
	if err != nil || code != "123456" {
		t.Fatalf("Code() = %q, %v", code, err)
	}
	if !strings.Contains(out.String(), "Phone") || !strings.Contains(out.String(), "Code") {
		t.Errorf("prompts not written to out: %q", out.String())
	}
}

func TestTermAuthSignUpUnsupported(t *testing.T) {
	if _, err := (termAuth{}).SignUp(context.Background()); err == nil {
		t.Error("expected SignUp to be unsupported")
	}
}
