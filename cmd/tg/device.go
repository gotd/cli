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

// deviceConfig mimics Telegram Desktop so the session shows up as a desktop
// client in Settings → Devices (mirrors tdl's tutil.Device).
func deviceConfig() telegram.DeviceConfig {
	return telegram.DeviceConfig{
		DeviceModel:    "Desktop",
		SystemVersion:  "Windows 10",
		AppVersion:     "4.2.4 x64",
		LangCode:       "en",
		SystemLangCode: "en-US",
		LangPack:       "tdesktop",
	}
}
