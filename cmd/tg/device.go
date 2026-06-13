package main

import (
	"strconv"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram"
)

// Built-in app credentials, injected at build time from the APP_ID/APP_HASH
// secrets via -ldflags "-X main.builtinAppID=... -X main.builtinAppHash=..."
// (see .goreleaser.yaml). They are empty in plain source builds, so app
// credentials are mandatory: release binaries carry them, and source builds
// must supply them per account (tg init/accounts add --app-id --app-hash, or
// APP_ID/APP_HASH env). There is intentionally no public fallback.
//
//nolint:gochecknoglobals // injected via -ldflags -X at build time
var (
	builtinAppID   = ""
	builtinAppHash = ""
)

// builtinCreds parses the build-time-injected app credentials. It errors when
// they are absent (a source build) or malformed, rather than falling back to
// any default.
func builtinCreds() (appID int, appHash string, err error) {
	if builtinAppID == "" || builtinAppHash == "" {
		return 0, "", errors.New("no app credentials: pass --app-id/--app-hash to `tg init` " +
			"or `tg accounts add` (get them from https://my.telegram.org), " +
			"or use a release binary that embeds them")
	}
	id, err := strconv.Atoi(builtinAppID)
	if err != nil {
		return 0, "", errors.Wrapf(err, "invalid built-in app id %q", builtinAppID)
	}
	return id, builtinAppHash, nil
}

// effectiveCreds resolves the app id/hash for an account: the user's own
// credentials if set, the gotd test-DC credentials on the test server (which
// the test phone numbers require), otherwise the build-time-injected built-in
// credentials. It errors when no credentials are available.
func effectiveCreds(acc Account, test bool) (appID int, appHash string, err error) {
	if acc.AppID != 0 && acc.AppHash != "" {
		return acc.AppID, acc.AppHash, nil
	}
	if test {
		return telegram.TestAppID, telegram.TestAppHash, nil
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
