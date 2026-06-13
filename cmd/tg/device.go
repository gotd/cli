package main

import (
	"strconv"

	"github.com/gotd/td/telegram"
)

// Built-in app credentials. The defaults are the public Telegram Desktop API
// id/hash (matching tdl's "desktop" app), used when the user has not configured
// their own app at https://my.telegram.org. Pairing them with the tdesktop
// device profile below makes the session appear as a legitimate desktop client.
//
// Release builds override these via -ldflags "-X main.builtinAppID=... -X
// main.builtinAppHash=..." from APP_ID/APP_HASH secrets (see .goreleaser.yaml),
// so they are vars (not consts) and builtinAppID is a string for ldflags.
//
// See: https://opentele.readthedocs.io/en/latest/documentation/authorization/api/
//
//nolint:gochecknoglobals // overridable via -ldflags -X at release time
var (
	builtinAppID   = "2040"
	builtinAppHash = "b18441a1ff607e10a989891a5462e627"
)

// builtinCreds parses the (possibly ldflags-injected) built-in credentials,
// falling back to the tdesktop defaults if the injected app id is not a valid
// integer.
func builtinCreds() (appID int, appHash string) {
	id, err := strconv.Atoi(builtinAppID)
	hash := builtinAppHash
	if err != nil || hash == "" {
		// Injected creds missing or malformed: use the tdesktop defaults.
		id, hash = 2040, "b18441a1ff607e10a989891a5462e627"
	}
	return id, hash
}

// effectiveCreds resolves the app id/hash for an account: the user's own
// credentials if set, otherwise the built-in Telegram Desktop credentials — or,
// on the test server, gotd's test-DC credentials (which the test phone numbers
// require).
func effectiveCreds(acc Account, test bool) (appID int, appHash string) {
	if acc.AppID != 0 && acc.AppHash != "" {
		return acc.AppID, acc.AppHash
	}
	if test {
		return telegram.TestAppID, telegram.TestAppHash
	}
	return builtinCreds()
}

// deviceConfig mimics Telegram Desktop (Windows) so the session shows up as a
// desktop client in Settings → Devices. It delegates to gotd's built-in preset,
// which keeps the app version in sync with the bundled tdesktop reference and
// sends the same initConnection params (including the tz_offset timezone field)
// as the real client, making the connection indistinguishable from Telegram
// Desktop's. Pair it with telegram.TDesktopResolver on the transport layer.
func deviceConfig() telegram.DeviceConfig {
	return telegram.DeviceTDesktopWindows()
}
