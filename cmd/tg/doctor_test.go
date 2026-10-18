package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type doctorEnvelope struct {
	Data  *doctorResult `json:"data"`
	Error *errorInfo    `json:"error"`
}

func parseDoctor(t *testing.T, stdout string) doctorResult {
	t.Helper()
	var env doctorEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || env.Data == nil {
		t.Fatalf("stdout is not a doctor result (%v):\n%s", err, stdout)
	}
	if env.Error != nil {
		t.Fatalf("doctor must report through its result, got an error envelope: %s", stdout)
	}
	return *env.Data
}

func checkByName(r doctorResult, name string) (doctorCheck, bool) {
	for _, c := range r.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return doctorCheck{}, false
}

func TestDoctorMissingConfig(t *testing.T) {
	code, stdout, stderr := runCLIConfig(t, filepath.Join(t.TempDir(), "missing.yaml"), "doctor", "-o", "json")
	if code != exitError || stderr != "" {
		t.Errorf("exit = %d, stderr = %q; want 1 and nothing on stderr", code, stderr)
	}
	res := parseDoctor(t, stdout)
	if c, ok := checkByName(res, "config"); !ok || c.OK || !strings.Contains(c.Hint, "tg init") {
		t.Errorf("config check = %+v", c)
	}
}

func TestDoctorNoSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gotd.cli.yaml")
	if err := os.WriteFile(path, []byte("app_id: 1\napp_hash: abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runCLIConfig(t, path, "doctor", "--offline", "-o", "json")
	if code != exitAuth {
		t.Errorf("exit = %d, want %d (not logged in)", code, exitAuth)
	}
	res := parseDoctor(t, stdout)
	if c, _ := checkByName(res, "credentials"); !c.OK || !strings.Contains(c.Detail, "from the config") {
		t.Errorf("credentials = %+v", c)
	}
	if c, _ := checkByName(res, "session"); c.OK || !strings.Contains(c.Hint, "tg login -a default") {
		t.Errorf("session = %+v", c)
	}
	if res.OK {
		t.Error("result should not be ok")
	}
}

func TestDoctorOfflineHealthy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gotd.cli.yaml")
	if err := os.WriteFile(path, []byte("app_id: 1\napp_hash: abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The session the default account would use, in gotd's format.
	saveTestSession(t, newSessionStore(dir, defaultAccount, Account{AppID: 1, AppHash: "abc"}, kindUser, false), 7, 1)

	code, stdout, _ := runCLIConfig(t, path, "doctor", "--offline", "-o", "json")
	res := parseDoctor(t, stdout)
	if code != exitOK || !res.OK {
		t.Fatalf("exit = %d, result = %+v", code, res)
	}
	for _, name := range []string{"version", "config", "account", "credentials", "session", "peer cache"} {
		if c, ok := checkByName(res, name); !ok || !c.OK {
			t.Errorf("check %q = %+v, %v", name, c, ok)
		}
	}
	if _, ok := checkByName(res, "connection"); ok {
		t.Error("--offline must skip the connection check")
	}
}
