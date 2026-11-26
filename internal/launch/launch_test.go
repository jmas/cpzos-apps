package launch

import (
	"errors"
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	argv, err := Expand(`player --name %c "two words" %%`, "P", "p.desktop", "", "/usr/share/applications/p.desktop")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"player", "--name", "P", "two words", "%"}
	if strings.Join(argv, "|") != strings.Join(want, "|") {
		t.Fatalf("got %#v, want %#v", argv, want)
	}
}

func TestExpandIcon(t *testing.T) {
	argv, err := Expand(`player %i`, "P", "p.desktop", "media-playback-start", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(argv, " ") != "player --icon media-playback-start" {
		t.Fatalf("got %#v", argv)
	}
	argv, err = Expand(`player %i`, "P", "p.desktop", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(argv, " ") != "player" {
		t.Fatalf("empty icon: got %#v", argv)
	}
}

func TestExpandRejectsDocuments(t *testing.T) {
	for _, code := range []string{"%f", "%F", "%u", "%U"} {
		if _, err := Expand("player "+code, "P", "p.desktop", "", ""); err == nil {
			t.Errorf("%s accepted", code)
		}
	}
}

func TestExpandRejectsUnknown(t *testing.T) {
	if _, err := Expand("player %q", "P", "p.desktop", "", ""); err == nil {
		t.Error("unknown field code accepted")
	}
	if _, err := Expand(`player "unterminated`, "P", "p.desktop", "", ""); err == nil {
		t.Error("unterminated quote accepted")
	}
}

func TestStartWaitSuccess(t *testing.T) {
	p, err := Start(Spec{Argv: []string{"true"}, Name: "True"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "True" {
		t.Fatalf("name = %q", p.Name())
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("clean exit reported %v", err)
	}
}

func TestStartWaitFailure(t *testing.T) {
	p, err := Start(Spec{Argv: []string{"false"}, Name: "False"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err == nil {
		t.Fatal("failed exit reported success")
	}
}

// One graphical entry is one app across every tab. The lock belongs to the
// running process, so it vanishes with it and no PID file can go stale.
func TestStartSingleInstance(t *testing.T) {
	t.Setenv("CPZ_RUN_DIR", t.TempDir())
	first, err := Start(Spec{Argv: []string{"sleep", "1"}, Name: "One", ID: "one.desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(Spec{Argv: []string{"true"}, Name: "One", ID: "one.desktop"}); !errors.Is(err, ErrRunning) {
		t.Fatalf("same entry while running = %v, want ErrRunning", err)
	}
	other, err := Start(Spec{Argv: []string{"true"}, Name: "Other", ID: "other.desktop"})
	if err != nil {
		t.Fatalf("different entry blocked: %v", err)
	}
	if err := other.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err != nil {
		t.Fatal(err)
	}
	again, err := Start(Spec{Argv: []string{"true"}, Name: "One", ID: "one.desktop"})
	if err != nil {
		t.Fatalf("entry not released after exit: %v", err)
	}
	if err := again.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestExpandEmpty(t *testing.T) {
	if _, err := Expand("   ", "P", "p.desktop", "", ""); err == nil {
		t.Error("empty Exec accepted")
	}
}
