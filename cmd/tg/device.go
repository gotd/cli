package main

import "github.com/gotd/td/telegram"

// Built-in app credentials, mirroring tdl: the public Telegram Desktop API
// id/hash. They are used when the user has not configured their own app at
// https://my.telegram.org. Pairing them with the tdesktop device profile below
// makes the session appear as a legitimate desktop client.
//
// See: https://opentele.readthedocs.io/en/latest/documentation/authorization/api/
const (
	builtinAppID   = 2040
	builtinAppHash = "b18441a1ff607e10a989891a5462e627"
)

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
	return builtinAppID, builtinAppHash
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
