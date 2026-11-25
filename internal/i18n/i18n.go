package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFS embed.FS

// Codes lists the supported product languages.
var Codes = []string{"en", "de", "es", "fr", "it", "ja", "pl", "pt", "tr", "uk", "zh"}

var supported = map[string]bool{
	"en": true, "de": true, "es": true, "fr": true, "it": true, "ja": true,
	"pl": true, "pt": true, "tr": true, "uk": true, "zh": true,
}

type file struct {
	Language string                     `json:"language"`
	Strings  map[string]json.RawMessage `json:"strings"`
}

type messages struct {
	text  map[string]string
	forms map[string]map[string]string
}

var english = mustLoad("en")

// Normalize maps a locale name to a supported UI language, or "".
func Normalize(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "_", "-")
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}
	if supported[s] {
		return s
	}
	return ""
}

// System resolves the UI language:
// CPZ_LANG, ~/.config/cpz/language, /etc/cpz/language, LANG/LC_ALL, en.
func System() string {
	var candidates []string
	candidates = append(candidates, os.Getenv("CPZ_LANG"))
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, readLine(filepath.Join(home, ".config/cpz/language")))
	}
	candidates = append(candidates,
		readLine("/etc/cpz/language"),
		os.Getenv("LC_ALL"),
		os.Getenv("LANG"),
	)
	for _, c := range candidates {
		if lang := Normalize(c); lang != "" {
			return lang
		}
	}
	return "en"
}

func readLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// Translator returns strings in one language with an English fallback.
type Translator struct {
	Lang string
	tag  language.Tag
	msg  messages
}

// For loads a language; an unknown language returns English.
func For(lang string) *Translator {
	lang = Normalize(lang)
	if lang == "" {
		lang = "en"
	}
	m := english
	if lang != "en" {
		if loaded, err := load(lang); err == nil {
			m = loaded
		}
	}
	return &Translator{Lang: lang, tag: language.Make(lang), msg: m}
}

// T substitutes {name} placeholders in pairs.
func (t *Translator) T(key string, pairs ...string) string {
	s, ok := t.msg.text[key]
	if !ok {
		s = english.text[key]
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		s = strings.ReplaceAll(s, "{"+pairs[i]+"}", pairs[i+1])
	}
	return s
}

// N selects a CLDR plural form.
func (t *Translator) N(key string, n int) string {
	form := pluralName(plural.Cardinal.MatchPlural(t.tag, n, 0, 0, 0, 0))
	if m, ok := t.msg.forms[key]; ok {
		if s, ok := m[form]; ok {
			return strings.ReplaceAll(s, "{n}", strconv.Itoa(n))
		}
		if s, ok := m["other"]; ok {
			return strings.ReplaceAll(s, "{n}", strconv.Itoa(n))
		}
	}
	if m, ok := english.forms[key]; ok {
		return strings.ReplaceAll(m["other"], "{n}", strconv.Itoa(n))
	}
	return t.T(key)
}

func pluralName(f plural.Form) string {
	switch f {
	case plural.Zero:
		return "zero"
	case plural.One:
		return "one"
	case plural.Two:
		return "two"
	case plural.Few:
		return "few"
	case plural.Many:
		return "many"
	default:
		return "other"
	}
}

func mustLoad(lang string) messages {
	m, err := load(lang)
	if err != nil {
		panic("i18n: " + err.Error())
	}
	return m
}

func load(lang string) (messages, error) {
	b, err := localeFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return messages{}, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return messages{}, err
	}
	out := messages{text: map[string]string{}, forms: map[string]map[string]string{}}
	for key, raw := range f.Strings {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			out.text[key] = s
			continue
		}
		var forms map[string]string
		if err := json.Unmarshal(raw, &forms); err == nil {
			out.forms[key] = forms
		}
	}
	return out, nil
}
