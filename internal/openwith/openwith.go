// Package openwith is the single entry point for opening a path. It picks a
// handler from the installed desktop entries, a small configuration file and a
// directory fallback, so an app never has to know which program owns a file.
package openwith

import (
	"bufio"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/launch"
)

// Handler is one installed application that can open a path.
type Handler struct {
	ID      string
	Name    string
	Entry   apps.Entry
	Default bool
}

// Candidates returns the handlers for path, best first. A directory always
// falls back to Terminal; a file needs an installed MimeType handler or a
// configured default.
func Candidates(path, lang string) []Handler {
	entries, _ := apps.LoadAll()
	byID := map[string]apps.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	dir := IsDir(path)
	key := Key(path)
	cfg := loadConfig()

	seen := map[string]bool{}
	var out []Handler
	add := func(e apps.Entry, def bool) {
		if seen[e.ID] {
			return
		}
		seen[e.ID] = true
		out = append(out, Handler{ID: e.ID, Name: e.Name(lang), Entry: e, Default: def})
	}

	if id := cfg[key]; id != "" {
		if e, ok := byID[id]; ok {
			add(e, true)
		}
	}
	for _, e := range entries {
		if supports(e, key) {
			add(e, false)
		}
	}
	if dir {
		if e, ok := byID["cpz-terminal.desktop"]; ok {
			add(e, false)
		}
	}
	if len(out) > 0 {
		hasDefault := false
		for _, h := range out {
			if h.Default {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			// The configured entry was missing. Prefer Terminal for
			// directories, otherwise the first MimeType match.
			for i := range out {
				if dir && out[i].ID == "cpz-terminal.desktop" {
					out[i].Default = true
					hasDefault = true
					break
				}
			}
			if !hasDefault {
				out[0].Default = true
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Default returns the preferred handler, if any.
func Default(path, lang string) (Handler, bool) {
	c := Candidates(path, lang)
	if len(c) == 0 {
		return Handler{}, false
	}
	return c[0], true
}

// Find returns one installed handler by desktop id, even if its MimeType does
// not advertise this path. It is used by `cpz-app open --with`.
func Find(id, path, lang string) (Handler, bool) {
	entries, _ := apps.LoadAll()
	for _, e := range entries {
		if e.ID == id {
			return Handler{ID: e.ID, Name: e.Name(lang), Entry: e}, true
		}
	}
	return Handler{}, false
}

// Spec builds the launch argv for a handler. A directory handler that is a
// terminal app runs with the directory as its working directory; a file
// handler gets the path through %f/%F/%u/%U.
func Spec(h Handler, path, lang string) (launch.Spec, error) {
	dir := IsDir(path)
	argv, err := launch.ExpandTarget(h.Entry.Exec(), h.Name, h.ID, h.Entry.Icon(), h.Entry.Path, path)
	if err != nil {
		return launch.Spec{}, err
	}
	spec := launch.Spec{
		Argv:     argv,
		Name:     h.Name,
		ID:       "openwith:" + h.ID,
		Path:     path,
		Terminal: h.Entry.Terminal(),
	}
	if dir && spec.Terminal {
		spec.Dir = path
	}
	return spec, nil
}

// IsDir reports whether path is a directory.
func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// Key is the configuration key for a path: "directory" or a MIME type.
func Key(path string) string {
	if IsDir(path) {
		return "directory"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if mt := mime.TypeByExtension(ext); mt != "" {
		if before, _, ok := strings.Cut(mt, ";"); ok {
			return strings.ToLower(strings.TrimSpace(before))
		}
		return strings.ToLower(strings.TrimSpace(mt))
	}
	return "application/octet-stream"
}

func supports(e apps.Entry, key string) bool {
	if key == "directory" {
		if e.Fields["X-CPZ-Open"] == "directory" {
			return true
		}
		key = "inode/directory"
	}
	for _, mt := range strings.Split(e.Fields["MimeType"], ";") {
		if strings.EqualFold(strings.TrimSpace(mt), key) {
			return true
		}
	}
	return false
}

// loadConfig merges /etc/cpz/open-with.conf and the user's file. Keys are
// "directory" or MIME types; values are desktop ids.
func loadConfig() map[string]string {
	out := map[string]string{}
	load := func(path string) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.TrimSpace(v)
			if k != "" && v != "" {
				out[k] = v
			}
		}
	}
	load("/etc/cpz/open-with.conf")
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir, _ = os.UserConfigDir()
	}
	if dir != "" {
		load(filepath.Join(dir, "cpz", "open-with.conf"))
	}
	return out
}
