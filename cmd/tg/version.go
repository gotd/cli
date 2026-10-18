package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// versionInfo describes the running binary, read from the build info Go embeds.
type versionInfo struct {
	Version    string `json:"version"`
	Commit     string `json:"commit,omitempty"`
	CommitTime string `json:"commit_time,omitempty"`
	Modified   bool   `json:"modified,omitempty"`
	Go         string `json:"go"`
	// BuiltinCredentials reports whether app credentials were embedded at
	// build time (their values are never printed).
	BuiltinCredentials bool `json:"builtin_credentials"`
}

func buildVersion() versionInfo {
	v := versionInfo{
		Version:            "(devel)",
		BuiltinCredentials: builtinAppID != "" && builtinAppHash != "",
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	v.Go = bi.GoVersion
	if bi.Main.Version != "" {
		v.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			v.Commit = s.Value
		case "vcs.time":
			v.CommitTime = s.Value
		case "vcs.modified":
			v.Modified = s.Value == "true"
		}
	}
	return v
}

// String renders a one-line version, e.g. "v0.12.0 (4bbb4ac, go1.25.11)".
func (v versionInfo) String() string {
	details := make([]string, 0, 2)
	if v.Commit != "" {
		c := v.Commit
		if len(c) > 12 {
			c = c[:12]
		}
		if v.Modified {
			c += "-dirty"
		}
		details = append(details, c)
	}
	if v.Go != "" {
		details = append(details, v.Go)
	}
	if len(details) == 0 {
		return v.Version
	}
	return v.Version + " (" + strings.Join(details, ", ") + ")"
}

// MarshalText renders the version line.
func (v versionInfo) MarshalText(w io.Writer) error {
	_, err := fmt.Fprintln(w, "tg", v.String())
	return err
}

// versionCmdName is the version command, which skipConfig exempts from loading a config.
const versionCmdName = "version"

func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     versionCmdName,
		Short:   "Print version and build information",
		GroupID: groupAuth,
		Long: `Print the version, commit and Go toolchain of this binary, and whether app
credentials were embedded at build time. Use it to confirm which build an agent
or script is running.`,
		Example: "  tg version\n  tg version -o json",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return a.printer.Emit(buildVersion())
		},
	}
}
