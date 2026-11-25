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

func selectMenuID(m *Model, id string) {
	m.menu.Cursor = 0
	for i, it := range m.menu.Items {
		if it.ID == id {
			m.menu.Cursor = i
			return
		}
	}
}

func TestMenuStructure(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	if m.view != viewMenu {
		t.Fatalf("view = %v, want menu", m.view)
	}
	ids := []string{}
	for _, it := range m.menu.Items {
		ids = append(ids, it.ID)
	}
	want := []string{itemUpdate, catInstalled, "AudioVideo", "Game"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("menu = %v, want %v", ids, want)
	}
	out := m.View()
	for _, s := range []string{"Оновити список", "Встановлені", "Аудіо та відео", "Ігри"} {
		if !strings.Contains(out, s) {
			t.Errorf("menu missing %q:\n%s", s, out)
		}
	}
	if w := tui.Width(strings.Split(out, "\n")[0]); w > 45 {
		t.Errorf("header too wide: %d", w)
	}
}

func TestEnterCategoryAndDetail(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	selectMenuID(&m, "AudioVideo")
	nm, _ := m.handleMenu(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewCategory {
		t.Fatalf("view = %v, want category", m.view)
	}
	if len(m.list.Items) != 1 || m.list.Items[0].Name != "Плеєр" {
		t.Fatalf("items = %#v", m.list.Items)
	}
	nm, _ = m.handleCategory(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewDetail {
		t.Fatalf("view = %v, want detail", m.view)
	}
	if !strings.Contains(m.View(), "Видалити") || !strings.Contains(m.View(), "Відкрити") {
		t.Errorf("actions missing:\n%s", m.View())
	}
	nm, _ = m.handleDetail(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.view != viewCategory {
		t.Fatalf("Esc in detail -> %v", m.view)
	}
	nm, _ = m.handleCategory(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.view != viewMenu {
		t.Fatalf("Esc in category -> %v", m.view)
	}
}

func TestEscExitsMenu(t *testing.T) {
	setup(t)
	m := New()
	_, cmd := m.handleMenu(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuit(cmd) {
		t.Fatal("Esc at the menu root did not exit")
	}
}

func TestSearchInsideCategory(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	m.enterCategory("AudioVideo")
	m.search.Active = true
	m.search.Value = "пле"
	m.query = "пле"
	m.applyCategoryFilter()
	if len(m.list.Items) != 1 || m.list.Items[0].Name != "Плеєр" {
		t.Fatalf("items = %#v", m.list.Items)
	}
}

func TestTypingStartsCategorySearch(t *testing.T) {
	setup(t)
	m := New()
	m.width, m.height = 45, 13
	m.enterCategory("AudioVideo")
	nm, _ := m.handleCategory(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	m = nm.(Model)
	if !m.search.Active || m.search.Value != "P" {
		t.Fatalf("search = active:%v value:%q", m.search.Active, m.search.Value)
	}
	nm, _ = m.handleCategory(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = nm.(Model)
	if m.search.Value != "Pf" {
		t.Fatalf("d-pad key while typing = %q", m.search.Value)
	}
}

func TestSystemAppHasNoRemove(t *testing.T) {
	setup(t)
	m := New()
	m.detail = apps.App{ID: "player.desktop", Name: "Player", Installed: true, System: true}
	m.buildActions()
	for _, it := range m.actions.Items {
		if it.ID == "remove" {
			t.Fatalf("system app offers remove: %#v", m.actions.Items)
		}
	}
}

func TestUpdateConfirmCancels(t *testing.T) {
	setup(t)
	m := New()
	selectMenuID(&m, itemUpdate)
	nm, _ := m.handleMenu(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.view != viewConfirm {
		t.Fatalf("view = %v, want confirm", m.view)
	}
	nm, _ = m.handleConfirm(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.view != viewMenu {
		t.Fatalf("Esc in confirm -> %v", m.view)
	}
}
