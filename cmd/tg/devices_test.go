package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
)

func TestNewDevicesResult(t *testing.T) {
	res := newDevicesResult(&tg.AccountAuthorizations{
		Authorizations: []tg.Authorization{
			{Hash: 1, Current: true, DeviceModel: "Desktop", APIID: 2040},
			{Hash: 2, DeviceModel: "iPhone", APIID: 6},
			{Hash: 3, DeviceModel: "Web", APIID: 2496},
		},
	})

	if res.Current == nil {
		t.Fatal("current session not detected")
	}
	if res.Current.Hash != 1 || res.Current.APIID != 2040 {
		t.Errorf("current = %+v", res.Current)
	}
	if len(res.Others) != 2 {
		t.Fatalf("got %d others, want 2", len(res.Others))
	}
	if res.Others[0].DeviceModel != "iPhone" || res.Others[1].DeviceModel != "Web" {
		t.Errorf("others order = %+v", res.Others)
	}
}

func TestDevicesResultMarshalText(t *testing.T) {
	res := newDevicesResult(&tg.AccountAuthorizations{
		Authorizations: []tg.Authorization{
			{
				Hash: 1, Current: true, DeviceModel: "Desktop", Platform: "Windows",
				SystemVersion: "10", AppName: clientDesktop, AppVersion: "5.0",
				APIID: 2040, IP: "1.2.3.4", Country: "Wonderland",
			},
			{Hash: 2, DeviceModel: "iPhone", APIID: 6},
		},
	})

	var buf bytes.Buffer
	if err := res.MarshalText(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{"This device:", "Desktop · Windows 10", "app_id 2040, official Telegram Desktop", "1.2.3.4 — Wonderland", "Active sessions (1):", "iPhone"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestRunTerminateSession(t *testing.T) {
	api, mock := newTestAPI(t)
	mock.ExpectCall(&tg.AccountResetAuthorizationRequest{Hash: 123456789}).
		ThenResult(&tg.BoolBox{Bool: &tg.BoolTrue{}})

	var buf bytes.Buffer
	p := output.New(output.Text, &buf)
	if err := runTerminateSession(context.Background(), api, 123456789, p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ok") {
		t.Errorf("output %q missing ok", buf.String())
	}
}

func TestOfficialAppDetection(t *testing.T) {
	// Known official api_id is flagged even when the server bit is unset.
	if info := newDeviceInfo(tg.Authorization{APIID: 6}); !info.OfficialApp {
		t.Errorf("api_id 6 (Telegram Android) not detected as official")
	}
	if name, ok := officialApp(2040); !ok || name != "Telegram Desktop" {
		t.Errorf("officialApp(2040) = %q, %v", name, ok)
	}
	// Unknown api_id falls back to the server's official_app flag.
	if info := newDeviceInfo(tg.Authorization{APIID: 999999}); info.OfficialApp {
		t.Errorf("unknown api_id should not be official without server flag")
	}
	if info := newDeviceInfo(tg.Authorization{APIID: 999999, OfficialApp: true}); !info.OfficialApp {
		t.Errorf("server official_app flag should be honored")
	}
}
