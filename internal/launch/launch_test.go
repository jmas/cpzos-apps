package launch

import (
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

func TestExpandEmpty(t *testing.T) {
	if _, err := Expand("   ", "P", "p.desktop", "", ""); err == nil {
		t.Error("empty Exec accepted")
	}
}
