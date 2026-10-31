package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/tg"
)

// signedURL builds a web view URL the way Telegram does: launch data in the fragment.
func signedURL(t *testing.T, page string) string {
	t.Helper()
	data := url.Values{
		"query_id":    {"AAH1"},
		"user":        {`{"id":42,"first_name":"Test","language_code":"en"}`},
		"auth_date":   {"1793400000"},
		"start_param": {"promo"},
		"hash":        {strings.Repeat("a", 64)},
	}
	fragment := url.Values{
		"tgWebAppData":     {data.Encode()},
		"tgWebAppVersion":  {"9.1"},
		"tgWebAppPlatform": {"tdesktop"},
	}
	return page + "#" + fragment.Encode()
}

func TestParseWebAppURL(t *testing.T) {
	launch, err := parseWebAppURL(signedURL(t, "https://store.example:8443/app?x=1"))
	require.NoError(t, err)
	require.Equal(t, "https://store.example:8443/app?x=1", launch.Page, "the fragment is gone")
	require.Equal(t, "9.1", launch.Version)
	require.Equal(t, "tdesktop", launch.Platform)
	require.EqualValues(t, 42, launch.UserID)
	require.EqualValues(t, 1793400000, launch.AuthDate)
	require.Equal(t, "AAH1", launch.QueryID)
	require.Equal(t, "promo", launch.StartParam)
	require.Contains(t, launch.InitData, "hash=")
}

func TestParseWebAppURLWithoutLaunchData(t *testing.T) {
	launch, err := parseWebAppURL("https://store.example/")
	require.NoError(t, err)
	require.Equal(t, "https://store.example/", launch.Page)
	require.Empty(t, launch.InitData)
}

func TestParseWebAppURLErrorsNeverQuoteTheCredential(t *testing.T) {
	secret := "hash=" + strings.Repeat("b", 64)
	for _, raw := range []string{
		"https://store.example/%zz#tgWebAppData=" + secret,
		"https://store.example/#tgWebAppData=%zz&" + secret,
	} {
		_, err := parseWebAppURL(raw)
		require.Error(t, err)
		require.NotContains(t, err.Error(), secret)
	}
}

func TestWebAppResultTextHidesTheCredential(t *testing.T) {
	launch, err := parseWebAppURL(signedURL(t, "https://store.example/"))
	require.NoError(t, err)
	res := webAppResult{
		Peer: peerRef{ID: 7, Username: "mybot"}, Source: "menu_button", URL: launch.Page,
		Signed: true, UserID: launch.UserID, AuthDate: launch.AuthDate,
	}
	var buf bytes.Buffer
	require.NoError(t, res.MarshalText(&buf))
	require.NotContains(t, buf.String(), "hash")
	require.Contains(t, buf.String(), "user 42")

	encoded, err := json.Marshal(res)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "signed_url", "without --reveal the JSON has no credential")
	require.NotContains(t, string(encoded), "init_data")
}

func TestWriteWebAppSecretIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := signedURL(t, "https://store.example/")
	launch, err := parseWebAppURL(raw)
	require.NoError(t, err)
	require.NoError(t, writeWebAppSecret(path, raw, launch))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var got webAppSecret
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, raw, got.URL)
	require.Equal(t, launch.InitData, got.InitData)
	require.Equal(t, "https://store.example/", got.Page)
}

func TestWebAppButtonURL(t *testing.T) {
	target, simple, ok := webAppButtonURL(&tg.KeyboardButtonWebView{URL: "https://a.example/"})
	require.True(t, ok)
	require.False(t, simple)
	require.Equal(t, "https://a.example/", target)

	target, simple, ok = webAppButtonURL(&tg.KeyboardButtonSimpleWebView{URL: "https://b.example/"})
	require.True(t, ok)
	require.True(t, simple)
	require.Equal(t, "https://b.example/", target)

	_, _, ok = webAppButtonURL(&tg.KeyboardButtonCallback{Data: []byte("x")})
	require.False(t, ok)
}
