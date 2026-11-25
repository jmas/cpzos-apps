package home

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jmas/cpzos-apps/internal/tui"
)

func setupApps(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "applications", "demo.desktop"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	t.Setenv("CPZ_CATALOG", filepath.Join(dir, "missing.json"))
	t.Setenv("CPZ_LANG", "uk")
}

func isQuit(cmd tea.Cmd) bool {
	return cmd != nil &&
		reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

func TestInstalledCatalogAppAppears(t *testing.T) {
	setupApps(t, `[Desktop Entry]
Type=Application
Name=Demo
Name[uk]=Демо
Exec=true
Terminal=true
Categories=Utility;
`)
	dir := os.Getenv("XDG_DATA_HOME")
	cat := filepath.Join(dir, "apps.json")
	if err := os.WriteFile(cat, []byte(`{"version":1,"generated":"2026-11-26T10:00:00Z","contract":1,"apps":[{"id":"demo.desktop","package":"cpz-app-demo","version":"1.0","name":{"en":"Demo","uk":"Демо"},"categories":["Utility"],"terminal":true,"system":false,"size":1,"contract":1}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_CATALOG", cat)
	m := New()
	m.width, m.height = 45, 13
	if out := m.View(); !strings.Contains(out, "Демо") {
		t.Fatalf("installed catalog app missing:\n%s", out)
	}
}

func TestShiftLetterStartsSearch(t *testing.T) {
	setupApps(t, `[Desktop Entry]
Type=Application
Name=Demo
Name[uk]=Демо
Exec=true
Terminal=true
Categories=Utility;
X-CPZ-System=true
`)
	m := New()
	m.width, m.height = 45, 13
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	m = nm.(Model)
	if !m.search.Active || m.search.Value != "M" {
		t.Fatalf("search = active:%v value:%q", m.search.Active, m.search.Value)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = nm.(Model)
	if m.search.Value != "Mf" {
		t.Fatalf("d-pad key while typing = %q", m.search.Value)
	}
}

func TestEscExitsAtRoot(t *testing.T) {
	setupApps(t, `[Desktop Entry]
Type=Application
Name=Demo
Name[uk]=Демо
Exec=true
Terminal=true
Categories=Utility;
X-CPZ-System=true
`)
	m := New()
	m.width, m.height = 45, 13
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuit(cmd) {
		t.Fatal("Esc at the Home root did not exit")
	}
}

func TestViewFitsPanel(t *testing.T) {
	setupApps(t, `[Desktop Entry]
Type=Application
Name=Demo
Name[uk]=Демо
Exec=true
Terminal=true
Categories=Utility;
X-CPZ-System=true
`)
	m := New()
	m.width, m.height = 45, 13
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 13 {
		t.Fatalf("got %d lines", len(lines))
	}
	for _, line := range lines {
		if w := tui.Width(line); w > 45 {
			t.Errorf("line too wide (%d): %q", w, line)
		}
	}
	if !strings.Contains(out, "Демо") {
		t.Errorf("app name missing:\n%s", out)
	}
}
