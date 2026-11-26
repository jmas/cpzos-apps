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

func TestMediaRootDirectSystemDir(t *testing.T) {
	home := t.TempDir()
	usb := t.TempDir()
	writeROM(t, filepath.Join(usb, "nes", "demo.nes"))
	games := ScanIn(home, []string{usb})
	if len(games) != 1 || games[0].System.ID != "nes" {
		t.Fatalf("games = %+v", games)
	}
}

func TestMediaRootLevelFile(t *testing.T) {
	home := t.TempDir()
	usb := t.TempDir()
	writeROM(t, filepath.Join(usb, "game.gba"))
	games := ScanIn(home, []string{usb})
	if len(games) != 1 || games[0].System.ID != "gba" {
		t.Fatalf("games = %+v", games)
	}
}

func TestAutomountRoot(t *testing.T) {
	home := t.TempDir()
	mnt := filepath.Join(t.TempDir(), "USB")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		t.Fatal(err)
	}
	writeROM(t, filepath.Join(mnt, "gba", "pocket.gba"))
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "sda1.mountpoint"), []byte(mnt+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_AUTOMOUNT_STATE", state)
	games := ScanIn(home, mediaRoots())
	if len(games) != 1 || games[0].System.ID != "gba" {
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
	if !strings.Contains(strings.Join(got, " "), "gba.input.SDLK.keyUp=9") {
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

// Mednafen's keyboard fields are SDL scancodes, not SDL key symbols. The
// Cardputer d-pad is F/X/Z/C, so the table must contain 9/27/29/6; 120/122/99
// are the old key symbols and leave three directions dead.
func TestMednafenConfigUsesScancodes(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "system", "emulators", "mednafen.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"command.exit keyboard 0x0 41",
		"nes.input.port1.gamepad.up keyboard 0x0 9",
		"nes.input.port1.gamepad.down keyboard 0x0 27",
		"nes.input.port1.gamepad.left keyboard 0x0 29",
		"nes.input.port1.gamepad.right keyboard 0x0 6",
		"nes.input.port1.gamepad.start keyboard 0x0 40",
		"nes.input.port1.gamepad.select keyboard 0x0 42",
		"snes_faust.renderer mt",
		"snes_faust.input.port1.gamepad.down keyboard 0x0 27",
		"sound.device hw:CARD=ES8389Audio,DEV=0",
		"sound.rate 32000",
		"video.driver sdl",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("mednafen.cfg is missing %q", want)
		}
	}
}

func TestSnesUsesFaust(t *testing.T) {
	snes, ok := ByID("snes")
	if !ok {
		t.Fatal("no snes system")
	}
	if snes.Module != "snes_faust" {
		t.Fatalf("snes module = %q, want snes_faust", snes.Module)
	}
	argv := strings.Join(snes.Argv("/roms/game.sfc"), " ")
	if !strings.Contains(argv, "-force_module snes_faust") {
		t.Fatalf("snes argv = %q", argv)
	}
}
