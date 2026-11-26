package pacman

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHelperArgs(t *testing.T) {
	cases := []struct {
		verb  string
		extra []string
		want  []string
	}{
		{"refresh-catalog", nil, []string{"refresh-catalog"}},
		{"refresh-catalog", []string{"--force"}, []string{"refresh-catalog", "--force"}},
		{"install", []string{"cpz-app-doom"}, []string{"install", "cpz-app-doom"}},
		{"remove", []string{"", "cpz-app-doom"}, []string{"remove", "cpz-app-doom"}},
	}
	for _, c := range cases {
		if got := helperArgs(c.verb, c.extra...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("helperArgs(%q, %v) = %v, want %v", c.verb, c.extra, got, c.want)
		}
	}
}

func TestRunPassesTheHelperArguments(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "app-install")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPZ_APP_INSTALL", helper)

	out, err := Run(context.Background(), "refresh-catalog", "--force", "")
	if err != nil {
		t.Fatalf("Run: %v (%s)", err, out)
	}
	if want := "refresh-catalog\n--force"; out != want {
		t.Fatalf("Run output = %q, want %q", out, want)
	}
}
