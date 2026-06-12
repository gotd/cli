package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgmock"

	"github.com/gotd/cli/internal/output"
)

// newTestAPI returns a tg.Client backed by a mock invoker.
func newTestAPI(t *testing.T) (*tg.Client, *tgmock.Mock) {
	t.Helper()
	mock := tgmock.NewRequire(t)
	return tg.NewClient(mock), mock
}

func TestRunWhoamiText(t *testing.T) {
	api, mock := newTestAPI(t)
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{
			ID:        42,
			Username:  "durov",
			FirstName: "Pavel",
			LastName:  "Durov",
			Phone:     "10000",
		},
	}})

	var buf bytes.Buffer
	p := output.New(output.Text, &buf)
	if err := runWhoami(context.Background(), api, p); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{"id=42", `name="Pavel Durov"`, "username=@durov", "phone=+10000"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q missing %q", got, want)
		}
	}
}

func TestRunWhoamiJSON(t *testing.T) {
	api, mock := newTestAPI(t)
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{ID: 7, Username: "bot", Bot: true},
	}})

	var buf bytes.Buffer
	p := output.New(output.JSON, &buf)
	if err := runWhoami(context.Background(), api, p); err != nil {
		t.Fatal(err)
	}

	var env struct {
		Schema int          `json:"schema"`
		Data   whoamiResult `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	if env.Schema != 1 {
		t.Errorf("schema = %d, want 1", env.Schema)
	}
	if env.Data.ID != 7 || !env.Data.Bot || env.Data.Username != "bot" {
		t.Errorf("unexpected data: %+v", env.Data)
	}
}

func TestRunWhoamiEmpty(t *testing.T) {
	api, mock := newTestAPI(t)
	mock.Expect().ThenResult(&tg.UserClassVector{Elems: nil})

	p := output.New(output.Text, &bytes.Buffer{})
	if err := runWhoami(context.Background(), api, p); err == nil {
		t.Fatal("expected error on empty users response")
	}
}
