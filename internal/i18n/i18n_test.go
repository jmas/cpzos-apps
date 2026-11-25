package i18n

import (
	"strings"
	"testing"
)

func TestUkrainianPlurals(t *testing.T) {
	tr := For("uk")
	cases := map[int]string{
		1:  "1 додаток",
		3:  "3 додатки",
		5:  "5 додатків",
		21: "21 додаток",
		22: "22 додатки",
		11: "11 додатків",
	}
	for n, want := range cases {
		if got := tr.N("apps.count", n); got != want {
			t.Errorf("N(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPlaceholders(t *testing.T) {
	tr := For("uk")
	got := tr.T("apps.confirm.remove", "name", "Плеєр")
	if got == "" || got == "apps.confirm.remove" {
		t.Fatalf("missing key: %q", got)
	}
	for _, want := range []string{"Плеєр", "пакет"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q does not contain %q", got, want)
		}
	}
}

func TestUnsupportedFallsBackToEnglish(t *testing.T) {
	tr := For("de")
	if got := tr.T("home.title"); got != "Home" {
		t.Errorf("de fallback = %q, want Home", got)
	}
	if tr.Lang != "de" {
		t.Errorf("lang = %q", tr.Lang)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"uk_UA.UTF-8": "uk",
		"en_US.UTF-8": "en",
		"zh-CN":       "zh",
		"de_DE":       "de",
		"C.UTF-8":     "",
		"ru_RU":       "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSystemEnv(t *testing.T) {
	t.Setenv("CPZ_LANG", "uk")
	if got := System(); got != "uk" {
		t.Errorf("System() = %q", got)
	}
	t.Setenv("CPZ_LANG", "bogus")
	t.Setenv("LANG", "en_US.UTF-8")
	// A user file on the host may still win; only assert that a supported code
	// comes back.
	if got := System(); got == "" {
		t.Error("System() returned no language")
	}
}
