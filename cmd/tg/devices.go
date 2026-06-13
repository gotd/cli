package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/output"
)

// clientDesktop is the official Telegram Desktop client name.
const clientDesktop = "Telegram Desktop"

// officialAppIDs maps every api_id Telegram Desktop recognizes as an official
// client to that client's name. The ids are lifted verbatim from tdesktop's
// settings_active_sessions.cpp (TypeFromEntry), which uses them to pick the
// official device icon. We use them to flag sessions as official even when the
// server's official_app bit is unset.
//
//nolint:gochecknoglobals // immutable lookup table, built once at init
var officialAppIDs = func() map[int]string {
	groups := []struct {
		name string
		ids  []int
	}{
		// Desktop (incl. the GitHub test build 17349 and Snap 611335).
		{clientDesktop, []int{2040, 17349, 611335}},
		{"Telegram macOS", []int{2834}},
		{"Telegram Android", []int{5, 6, 24, 1026, 1083, 2458, 2521, 21724}},
		{"Telegram iOS", []int{1, 7, 10840, 16352}},
		{"Telegram Web", []int{2496, 739222, 1025907}},
	}
	m := make(map[int]string)
	for _, g := range groups {
		for _, id := range g.ids {
			m[id] = g.name
		}
	}
	return m
}()

// officialApp reports the official client name for an api_id, if known.
func officialApp(apiID int) (string, bool) {
	name, ok := officialAppIDs[apiID]
	return name, ok
}

// deviceInfo describes one logged-in session (Telegram "device"), mirroring the
// fields shown in Telegram Desktop's Settings → Devices.
type deviceInfo struct {
	Hash          int64  `json:"hash"`
	Current       bool   `json:"current"`
	OfficialApp   bool   `json:"official_app"`
	DeviceModel   string `json:"device_model,omitempty"`
	Platform      string `json:"platform,omitempty"`
	SystemVersion string `json:"system_version,omitempty"`
	AppName       string `json:"app_name,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	APIID         int    `json:"app_id"`
	IP            string `json:"ip,omitempty"`
	Country       string `json:"country,omitempty"`
	Region        string `json:"region,omitempty"`
	DateCreated   int    `json:"date_created,omitempty"`
	DateActive    int    `json:"date_active,omitempty"`
}

// devicesResult is the stable result of `tg devices`: the current session
// followed by every other active session.
type devicesResult struct {
	Current *deviceInfo  `json:"current,omitempty"`
	Others  []deviceInfo `json:"others"`
}

// newDeviceInfo maps a tg.Authorization to our stable shape.
func newDeviceInfo(a tg.Authorization) deviceInfo {
	_, knownOfficial := officialApp(a.APIID)
	return deviceInfo{
		Hash:          a.Hash,
		Current:       a.Current,
		OfficialApp:   a.OfficialApp || knownOfficial,
		DeviceModel:   a.DeviceModel,
		Platform:      a.Platform,
		SystemVersion: a.SystemVersion,
		AppName:       a.AppName,
		AppVersion:    a.AppVersion,
		APIID:         a.APIID,
		IP:            a.IP,
		Country:       a.Country,
		Region:        a.Region,
		DateCreated:   a.DateCreated,
		DateActive:    a.DateActive,
	}
}

// newDevicesResult splits the authorizations into the current session and the
// rest, preserving the server's ordering for the others.
func newDevicesResult(auths *tg.AccountAuthorizations) devicesResult {
	var res devicesResult
	for _, a := range auths.Authorizations {
		info := newDeviceInfo(a)
		if a.Current {
			cur := info
			res.Current = &cur
			continue
		}
		res.Others = append(res.Others, info)
	}
	return res
}

// activeText renders a last-active timestamp the way Telegram Desktop does:
// "online" when recent, otherwise a short relative/absolute form.
func activeText(unix int) string {
	if unix == 0 {
		return ""
	}
	t := time.Unix(int64(unix), 0)
	d := time.Since(t)
	switch {
	case d < 2*time.Minute:
		return "online"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hr ago", int(d.Hours()))
	default:
		return t.Format("2006-01-02 15:04")
	}
}

// writeDevice renders one device as an indented block.
func writeDevice(w io.Writer, d deviceInfo) error {
	title := strings.TrimSpace(d.DeviceModel)
	if title == "" {
		title = "Unknown device"
	}
	sys := strings.TrimSpace(d.Platform + " " + d.SystemVersion)
	if sys != "" {
		title += " · " + sys
	}
	if _, err := fmt.Fprintf(w, "  %s\n", title); err != nil {
		return err
	}

	app := strings.TrimSpace(d.AppName + " " + d.AppVersion)
	if app == "" {
		app = "unknown app"
	}
	app += fmt.Sprintf(" (app_id %d", d.APIID)
	if name, ok := officialApp(d.APIID); ok {
		app += ", official " + name
	} else if d.OfficialApp {
		app += ", official"
	}
	app += ")"
	if _, err := fmt.Fprintf(w, "    %s\n", app); err != nil {
		return err
	}

	loc := d.IP
	if place := strings.TrimSpace(strings.Trim(d.Country+", "+d.Region, ", ")); place != "" {
		if loc != "" {
			loc += " — "
		}
		loc += place
	}
	if loc != "" {
		if _, err := fmt.Fprintf(w, "    %s\n", loc); err != nil {
			return err
		}
	}

	if active := activeText(d.DateActive); active != "" {
		if _, err := fmt.Fprintf(w, "    last active: %s\n", active); err != nil {
			return err
		}
	}
	return nil
}

// MarshalText renders the current device and active sessions like Telegram
// Desktop's Devices screen.
func (r devicesResult) MarshalText(w io.Writer) error {
	if r.Current != nil {
		if _, err := fmt.Fprintln(w, "This device:"); err != nil {
			return err
		}
		if err := writeDevice(w, *r.Current); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "\nActive sessions (%d):\n", len(r.Others)); err != nil {
		return err
	}
	for _, d := range r.Others {
		if err := writeDevice(w, d); err != nil {
			return err
		}
	}
	return nil
}

// runDevices fetches the active authorizations and emits them. It takes only an
// API client and printer so it is unit-testable with a mock invoker.
func runDevices(ctx context.Context, api *tg.Client, p *output.Printer) error {
	auths, err := api.AccountGetAuthorizations(ctx)
	if err != nil {
		return errors.Wrap(err, "account.getAuthorizations")
	}
	return p.Emit(newDevicesResult(auths))
}

func (a *app) newDevicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "devices",
		Short:   "List active sessions (logged-in devices)",
		GroupID: groupAuth,
		Long: `List every active session on the account, like Telegram's Settings → Devices:
the current device first, then all other logged-in sessions. Each line shows the
device, app (with its app_id in plaintext), location, and last-active time.`,
		Example: `  tg devices
  tg devices --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				return runDevices(ctx, api, a.printer)
			})
		},
	}
	return cmd
}
