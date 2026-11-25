package apps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmas/cpzos-apps/internal/catalog"
)

func writeEntry(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAll(t *testing.T) {
	data := t.TempDir()
	appsDir := filepath.Join(data, "applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	t.Setenv("XDG_CURRENT_DESKTOP", "CPZ")

	writeEntry(t, appsDir, "player.desktop", `[Desktop Entry]
Type=Application
Name=Player
Name[uk]=Плеєр
Comment=Play music
Comment[uk]=Відтворення музики
Exec=player
Terminal=true
Categories=AudioVideo;Player;
Keywords=music;audio;
Keywords[uk]=музика;
X-CPZ-System=false
`)
	writeEntry(t, appsDir, "hidden.desktop", "[Desktop Entry]\nType=Application\nName=Hidden\nExec=hidden\nNoDisplay=true\n")
	writeEntry(t, appsDir, "bad.desktop", "[Desktop Entry]\nType=Application\nName=NoExec\n")

	list, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d entries, want 1", len(list))
	}
	e := list[0]
	if e.Name("uk") != "Плеєр" || e.Name("en") != "Player" || e.Name("de") != "Player" {
		t.Errorf("names: uk=%q en=%q de=%q", e.Name("uk"), e.Name("en"), e.Name("de"))
	}
	if e.Comment("uk") != "Відтворення музики" {
		t.Errorf("comment uk = %q", e.Comment("uk"))
	}
	if !e.Terminal() {
		t.Error("Terminal not parsed")
	}
	if got := e.Keywords("uk"); len(got) != 1 || got[0] != "музика" {
		t.Errorf("keywords = %#v", got)
	}
	if got := e.Categories(); len(got) != 2 || got[0] != "AudioVideo" {
		t.Errorf("categories = %#v", got)
	}
}

func TestMergeInstalledWins(t *testing.T) {
	entry := Entry{ID: "player.desktop", Path: "/x/player.desktop", Fields: map[string]string{
		"Name": "Local player", "Exec": "player", "Terminal": "true",
	}}
	idx := &catalog.Index{Version: 1, Apps: []catalog.App{
		{ID: "player.desktop", Package: "cpz-app-player", Version: "0.4.1", Name: catalog.Localized{"en": "Player"}, Categories: []string{"AudioVideo"}},
		{ID: "other.desktop", Package: "cpz-app-other", Version: "1.0", Name: catalog.Localized{"en": "Other"}, Categories: []string{"Game"}},
	}}
	list := Build("en", []Entry{entry}, idx)
	if len(list) != 2 {
		t.Fatalf("got %d apps", len(list))
	}
	var player, other App
	for _, a := range list {
		switch a.ID {
		case "player.desktop":
			player = a
		case "other.desktop":
			other = a
		}
	}
	if !player.Installed || player.Package != "cpz-app-player" || player.Name != "Local player" {
		t.Errorf("player = %+v", player)
	}
	if other.Installed || other.Version != "1.0" {
		t.Errorf("other = %+v", other)
	}
}

func TestUnlistedInfrastructureHidden(t *testing.T) {
	installed := []Entry{
		{ID: "foot.desktop", Fields: map[string]string{"Name": "Foot", "Exec": "foot", "Categories": "System;"}},
		{ID: "terminal.desktop", Fields: map[string]string{"Name": "Terminal", "Exec": "bash", "X-CPZ-System": "true"}},
	}
	list := Build("en", installed, &catalog.Index{Version: 1})
	if len(list) != 1 || list[0].ID != "terminal.desktop" {
		t.Fatalf("list = %#v", list)
	}
}

func TestMainCategory(t *testing.T) {
	if got := MainCategory([]string{"Player", "AudioVideo"}); got != "AudioVideo" {
		t.Errorf("got %q", got)
	}
	if got := MainCategory([]string{"Custom"}); got != "Custom" {
		t.Errorf("got %q", got)
	}
}
