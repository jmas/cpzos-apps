package openwith

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDesktop(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "applications", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupApps(t *testing.T) (string, string) {
	t.Helper()
	data := t.TempDir()
	writeDesktop(t, data, "cpz-terminal.desktop", `[Desktop Entry]
Type=Application
Name=Terminal
Name[uk]=Термінал
Exec=bash -l
Terminal=true
Categories=System;TerminalEmulator;
MimeType=inode/directory;
`)
	writeDesktop(t, data, "cpz-app-mc.desktop", `[Desktop Entry]
Type=Application
Name=Midnight Commander
Name[uk]=Midnight Commander
Exec=/usr/bin/mc
Terminal=true
Categories=Utility;FileManager;
MimeType=inode/directory;
`)
	writeDesktop(t, data, "cpz-app-editor.desktop", `[Desktop Entry]
Type=Application
Name=Editor
Name[uk]=Редактор
Exec=editor %f
Terminal=true
Categories=Utility;
MimeType=text/plain;
`)
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	return data, config
}

func TestDirectoryCandidatesDefaultTerminal(t *testing.T) {
	setupApps(t)
	dir := t.TempDir()
	c := Candidates(dir, "en")
	if len(c) != 2 {
		t.Fatalf("candidates = %#v", c)
	}
	if c[0].ID != "cpz-terminal.desktop" || !c[0].Default {
		t.Fatalf("default = %#v", c[0])
	}
	spec, err := Spec(c[0], dir, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Terminal || spec.Dir != dir {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestDirectoryConfigOverride(t *testing.T) {
	_, config := setupApps(t)
	if err := os.MkdirAll(filepath.Join(config, "cpz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "cpz", "open-with.conf"),
		[]byte("directory=cpz-app-mc.desktop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c := Candidates(dir, "en")
	if len(c) == 0 || c[0].ID != "cpz-app-mc.desktop" || !c[0].Default {
		t.Fatalf("candidates = %#v", c)
	}
}

func TestFileMimeHandler(t *testing.T) {
	setupApps(t)
	file := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := Candidates(file, "en")
	if len(c) != 1 || c[0].ID != "cpz-app-editor.desktop" {
		t.Fatalf("candidates = %#v", c)
	}
	spec, err := Spec(c[0], file, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(spec.Argv, " ") != "editor "+file {
		t.Fatalf("argv = %#v", spec.Argv)
	}
}
