package roms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeROM(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanIn(t *testing.T) {
	home := t.TempDir()
	writeROM(t, filepath.Join(home, "roms", "nes", "demo.nes"))
	writeROM(t, filepath.Join(home, "roms", "nes", "sub", "another.NES"))
	writeROM(t, filepath.Join(home, "roms", "gb", "tetris.gbc"))
	writeROM(t, filepath.Join(home, "roms", "gba", "pocket.gba"))
	writeROM(t, filepath.Join(home, "roms", "nes", "notes.txt"))

	games := ScanIn(home, nil)
	if len(games) != 4 {
		t.Fatalf("got %d games, want 4: %+v", len(games), games)
	}
	counts := Counts(games)
	if counts["nes"] != 2 || counts["gb"] != 1 || counts["gba"] != 1 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestMediaRoot(t *testing.T) {
	home := t.TempDir()
	usb := t.TempDir()
	writeROM(t, filepath.Join(usb, "roms", "snes", "star.sfc"))
	games := ScanIn(home, []string{usb})
	if len(games) != 1 || games[0].System.ID != "snes" {
		t.Fatalf("games = %+v", games)
	}
}

func TestArgv(t *testing.T) {
	nes, _ := ByID("nes")
	got := nes.Argv("/roms/demo.nes")
	want := []string{"mednafen", "-ovconfig", MednafenConfig, "-force_module", "nes", "/roms/demo.nes"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("nes argv = %#v", got)
	}
	gba, _ := ByID("gba")
	got = gba.Argv("/roms/demo.gba")
	if got[0] != "mgba" || got[1] != "-f" || got[len(got)-1] != "/roms/demo.gba" {
		t.Fatalf("gba argv = %#v", got)
	}
	if !strings.Contains(strings.Join(got, " "), "gba.input.SDLK.keyUp=102") {
		t.Fatalf("gba argv has no Cardputer d-pad binding: %#v", got)
	}
}

func TestModelShowsGames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CPZ_LANG", "en")
	writeROM(t, filepath.Join(home, "roms", "nes", "demo.nes"))

	m := New("nes")
	if out := m.View(); !strings.Contains(out, "demo") {
		t.Fatalf("nes view missing demo:\n%s", out)
	}
	m = New("")
	if out := m.View(); !strings.Contains(out, "NES") {
		t.Fatalf("system view missing NES:\n%s", out)
	}
}
