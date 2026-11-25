package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/tui"
)

func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry := `[Desktop Entry]
Type=Application
Name=Player
Name[uk]=Плеєр
Comment=Play music
Exec=player
Terminal=true
Categories=AudioVideo;
`
	if err := os.WriteFile(filepath.Join(dir, "applications", "player.desktop"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := `{"version":1,"generated":"2026-11-26T10:00:00Z","contract":1,"apps":[
	  {"id":"player.desktop","package":"cpz-app-player","version":"1.0",
	   "name":{"en":"Player","uk":"Плеєр"},"comment":{"en":"Play music"},
	   "categories":["AudioVideo"],"terminal":true,"system":false,"size":1048576,"contract":1},
	  {"id":"remote.desktop","package":"cpz-app-remote","version":"1.0",
	   "name":{"en":"Remote","uk":"Віддалений"},"comment":{"en":"Remote app"},
	   "categories":["Game"],"terminal":true,"system":false,"size":1048576,"contract":1}]}`
	if err := os.WriteFile(filepath.Join(dir, "apps.json"), []byte(cat), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	t.Setenv("CPZ_CATALOG", filepath.Join(dir, "apps.json"))
	t.Setenv("CPZ_NO_REFRESH", "1")
	t.Setenv("CPZ_LANG", "uk")
}

func isQuit(cmd tea.Cmd) bool {
	return cmd != nil &&
		reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

func TestEscExitsListAndReturnsFromDetail(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuit(cmd) {
		t.Fatal("Esc at the list root did not exit")
	}
	m.selectID("player.desktop")
	nm, _ := m.openSelected()
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.view != viewList {
		t.Fatalf("Esc in detail left view %v", m.view)
	}
}

func TestListAndDetail(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	out := m.View()
	if len(strings.Split(out, "\n")) != 13 {
		t.Fatalf("list lines:\n%s", out)
	}
	if !strings.Contains(out, "Плеєр") || !strings.Contains(out, "Віддалений") {
		t.Errorf("apps missing:\n%s", out)
	}
	if !strings.Contains(out, "Оновити список") {
		t.Errorf("update row missing:\n%s", out)
	}
	if w := tui.Width(strings.Split(out, "\n")[0]); w > 45 {
		t.Errorf("header too wide: %d", w)
	}

	// Categories: All, Installed, System, Audio & Video, Games.
	if len(m.cats) != 5 {
		t.Fatalf("categories = %#v", m.cats)
	}
	// Focus the installed Player and open the detail screen.
	m.selectID("player.desktop")
	nm, _ := m.openSelected()
	m = nm.(Model)
	if m.view != viewDetail {
		t.Fatalf("view = %v, detail = %+v", m.view, m.detail)
	}
	out = m.View()
	if !strings.Contains(out, "Видалити") || !strings.Contains(out, "Відкрити") {
		t.Errorf("actions missing:\n%s", out)
	}
}

func TestSystemAppHasNoRemove(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	m.detail = apps.App{ID: "player.desktop", Name: "Player", Installed: true, System: true}
	m.buildActions()
	for _, it := range m.actions.Items {
		if it.ID == "remove" {
			t.Fatalf("system app offers remove: %#v", m.actions.Items)
		}
	}
}

func TestSearchFilter(t *testing.T) {
	setup(t)
	m := New()
	m.search.Active = true
	m.search.Value = "від"
	m.query = "від"
	m.applyFilter()
	if len(m.list.Items) != 1 {
		t.Fatalf("items = %#v", m.list.Items)
	}
}
