package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmas/cpzos-apps/internal/i18n"
)

func TestReorder(t *testing.T) {
	got := reorder([]string{"cpz-app-x", "--yes", "--json"}, nil)
	want := []string{"--yes", "--json", "cpz-app-x"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = reorder([]string{"--category", "Game", "cpz-app-x", "--json"},
		map[string]bool{"category": true})
	want = []string{"--category", "Game", "--json", "cpz-app-x"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestValidName(t *testing.T) {
	for _, s := range []string{"cpz-app-demo", "a", "x@y.z+1"} {
		if !validName(s) {
			t.Errorf("%q rejected", s)
		}
	}
	for _, s := range []string{"", "-x", "--x", "a b", "a/b", "a;b"} {
		if validName(s) {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestSettingsUpdateJSON(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	helper := filepath.Join(dir, "app-install")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CPZ_ARGS_LOG\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_APP_INSTALL", helper)
	t.Setenv("CPZ_ARGS_LOG", logPath)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := settingsUpdate([]string{"--json", "--yes"}, i18n.For("uk"))
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, out)
	}
	args, _ := os.ReadFile(logPath)
	if got, want := string(args), "update-system\n"; got != want {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
	var res opJSON
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !res.OK || res.Action != "update" {
		t.Fatalf("payload = %+v", res)
	}
}

func TestAppsUpdateJSON(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	helper := filepath.Join(dir, "app-install")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CPZ_ARGS_LOG\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_APP_INSTALL", helper)
	t.Setenv("CPZ_ARGS_LOG", logPath)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := appsOp("update", []string{"--json", "--yes"}, i18n.For("en"))
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, out)
	}
	args, _ := os.ReadFile(logPath)
	if got, want := string(args), "update-all\n"; got != want {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
	var res opJSON
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !res.OK || res.Action != "update" {
		t.Fatalf("payload = %+v", res)
	}
}

func TestAppsListJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry := `[Desktop Entry]
Type=Application
Name=Demo
Name[uk]=Демо
Comment=x
Exec=true
Terminal=true
Categories=Utility;
X-CPZ-System=true
`
	if err := os.WriteFile(filepath.Join(dir, "applications", "demo.desktop"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	t.Setenv("CPZ_CATALOG", filepath.Join(dir, "none.json"))

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := appsList([]string{"--json", "--installed"}, i18n.For("uk"))
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var payload listJSON
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(payload.Apps) != 1 || payload.Apps[0].Name != "Демо" || !payload.Apps[0].Installed {
		t.Fatalf("payload = %+v", payload)
	}
}
