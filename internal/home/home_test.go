package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
