package apps

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one XDG desktop entry.
type Entry struct {
	ID     string
	Path   string
	Fields map[string]string
}

// Parse reads the [Desktop Entry] group of a desktop file.
func Parse(r io.Reader) (map[string]string, error) {
	fields := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	inGroup := false
	sawGroup := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inGroup = line == "[Desktop Entry]"
			if inGroup {
				sawGroup = true
			}
			continue
		}
		if !inGroup {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(k)] = Unescape(strings.TrimSpace(v))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !sawGroup {
		return nil, os.ErrInvalid
	}
	return fields, nil
}

// Unescape expands the backslash escapes defined by the Desktop Entry spec.
func Unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	esc := false
	for _, r := range s {
		if esc {
			switch r {
			case 's':
				b.WriteByte(' ')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case ';':
				b.WriteByte(';')
			default:
				b.WriteRune(r)
			}
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		b.WriteRune(r)
	}
	if esc {
		b.WriteByte('\\')
	}
	return b.String()
}

// Get returns a localized field: exact language, then English, then raw.
func (e Entry) Get(key, lang string) string {
	if v := e.Fields[key+"["+lang+"]"]; v != "" {
		return v
	}
	if lang != "en" {
		if v := e.Fields[key+"[en]"]; v != "" {
			return v
		}
	}
	return e.Fields[key]
}

func (e Entry) Name(lang string) string    { return e.Get("Name", lang) }
func (e Entry) Comment(lang string) string { return e.Get("Comment", lang) }
func (e Entry) Icon() string               { return e.Fields["Icon"] }
func (e Entry) Exec() string               { return e.Fields["Exec"] }
func (e Entry) Terminal() bool             { return e.Fields["Terminal"] == "true" }
func (e Entry) System() bool               { return e.Fields["X-CPZ-System"] == "true" }

// Categories returns the categories of the entry.
func (e Entry) Categories() []string {
	raw := e.Get("Categories", "en")
	var out []string
	for _, c := range strings.Split(raw, ";") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// Keywords returns the localized search words.
func (e Entry) Keywords(lang string) []string {
	raw := e.Get("Keywords", lang)
	var out []string
	for _, k := range strings.Split(raw, ";") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// Admissible applies the visibility rules of APP-CONTRACT.md.
func (e Entry) Admissible() bool {
	if e.Fields["Type"] != "Application" {
		return false
	}
	if e.Fields["Hidden"] == "true" || e.Fields["NoDisplay"] == "true" {
		return false
	}
	if v := e.Fields["OnlyShowIn"]; v != "" && !contains(v, "CPZ") {
		return false
	}
	if v := e.Fields["NotShowIn"]; contains(v, "CPZ") {
		return false
	}
	return e.Fields["Name"] != "" && e.Fields["Exec"] != ""
}

func contains(list, want string) bool {
	for _, v := range strings.Split(list, ";") {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

// XDGDirs returns the application directories in precedence order.
func XDGDirs() []string {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		if home, err := os.UserHomeDir(); err == nil {
			data = filepath.Join(home, ".local/share")
		}
	}
	var bases []string
	if data != "" {
		bases = append(bases, data)
	}
	dirs := os.Getenv("XDG_DATA_DIRS")
	if dirs == "" {
		dirs = "/usr/local/share:/usr/share"
	}
	bases = append(bases, strings.Split(dirs, ":")...)

	var out []string
	seen := map[string]bool{}
	for _, b := range bases {
		if b == "" {
			continue
		}
		d := filepath.Join(b, "applications")
		if seen[d] {
			continue
		}
		seen[d] = true
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// LoadAll reads every admissible entry; a later directory wins on the same ID.
func LoadAll() ([]Entry, error) {
	byID := map[string]Entry{}
	for _, dir := range XDGDirs() {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range ents {
			if de.IsDir() || !strings.HasSuffix(de.Name(), ".desktop") {
				continue
			}
			path := filepath.Join(dir, de.Name())
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			fields, err := Parse(f)
			f.Close()
			if err != nil {
				continue
			}
			e := Entry{ID: de.Name(), Path: path, Fields: fields}
			if !e.Admissible() {
				if _, ok := byID[e.ID]; ok {
					delete(byID, e.ID) // a user override can hide a system entry
				}
				continue
			}
			byID[e.ID] = e
		}
	}
	out := make([]Entry, 0, len(byID))
	for _, e := range byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
