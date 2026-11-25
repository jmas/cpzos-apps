// Command build-catalog generates the app catalog from built pacman packages.
//
//	go run ./tools/build-catalog -packages dist -out apps.json
//
// It reads every .pkg.tar.* file with bsdtar, takes the desktop entries from
// /usr/share/applications and writes one schema-1 JSON file. The catalog is
// data only: it is packaged by cpz-apps-catalog and baked into the UI image.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmas/cpzos-apps/internal/apps"
	"github.com/jmas/cpzos-apps/internal/catalog"
	"github.com/jmas/cpzos-apps/internal/i18n"
)

func main() {
	packages := flag.String("packages", "dist", "directory with built .pkg.tar.* files")
	out := flag.String("out", "apps.json", "output catalog path")
	schema := flag.Int("schema", 1, "catalog schema version")
	flag.Parse()

	if err := run(*packages, *out, *schema); err != nil {
		fmt.Fprintln(os.Stderr, "build-catalog:", err)
		os.Exit(1)
	}
}

func run(dir, out string, schema int) error {
	if _, err := exec.LookPath("bsdtar"); err != nil {
		return errors.New("bsdtar is required (libarchive)")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var found []catalog.App
	seen := map[string]string{}
	for _, de := range ents {
		name := de.Name()
		if de.IsDir() || !strings.Contains(name, ".pkg.tar") || strings.HasSuffix(name, ".sig") {
			continue
		}
		pkgs, err := packageApps(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, a := range pkgs {
			if prev, ok := seen[a.ID]; ok {
				return fmt.Errorf("duplicate desktop id %s in %s and %s", a.ID, prev, name)
			}
			seen[a.ID] = name
			found = append(found, a)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })

	idx := catalog.Index{
		Version:   schema,
		Generated: generatedAt(),
		Contract:  1,
		Apps:      found,
	}
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(out, b, 0o644)
}

func generatedAt() time.Time {
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(secs, 0).UTC()
		}
	}
	return time.Now().UTC().Truncate(time.Second)
}

func packageApps(path string) ([]catalog.App, error) {
	info, err := readPKGINFO(path)
	if err != nil {
		return nil, err
	}
	list, err := bsdtar(path, "-tf")
	if err != nil {
		return nil, err
	}
	var out []catalog.App
	for _, line := range strings.Split(list, "\n") {
		p := strings.TrimSpace(line)
		if !strings.HasPrefix(p, "usr/share/applications/") || !strings.HasSuffix(p, ".desktop") {
			continue
		}
		content, err := bsdtar(path, "-xOf", p)
		if err != nil {
			return nil, err
		}
		fields, err := apps.Parse(bytes.NewReader([]byte(content)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		e := apps.Entry{ID: filepath.Base(p), Path: "/" + p, Fields: fields}
		if !e.Admissible() {
			continue
		}
		out = append(out, buildApp(info, e))
	}
	return out, nil
}

type pkgInfo struct {
	Name    string
	Version string
	Size    int64
}

func readPKGINFO(path string) (pkgInfo, error) {
	s, err := bsdtar(path, "-xOf", ".PKGINFO")
	if err != nil {
		return pkgInfo{}, err
	}
	return parsePKGINFO(s)
}

func parsePKGINFO(s string) (pkgInfo, error) {
	var info pkgInfo
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "pkgname":
			info.Name = strings.TrimSpace(v)
		case "pkgver":
			info.Version = strings.TrimSpace(v)
		case "size":
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				info.Size = n
			}
		}
	}
	if info.Name == "" {
		return info, errors.New(".PKGINFO has no pkgname")
	}
	return info, nil
}

func bsdtar(path string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("bsdtar: no mode")
	}
	// The archive path must follow -f, so place it before patterns.
	full := append([]string{args[0], path}, args[1:]...)
	cmd := exec.Command("bsdtar", full...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bsdtar %v: %v: %s", args, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func buildApp(info pkgInfo, e apps.Entry) catalog.App {
	name := localized(e, "Name")
	comment := localized(e, "Comment")
	a := catalog.App{
		ID:         e.ID,
		Package:    info.Name,
		Version:    info.Version,
		Name:       name,
		Comment:    comment,
		Keywords:   localizedKeywords(e),
		Categories: e.Categories(),
		Terminal:   e.Terminal(),
		System:     e.System() || info.Name == "cpz-system-apps",
		Size:       info.Size,
		Icon:       e.Icon(),
		Contract:   1,
	}
	if v := e.Fields["X-CPZ-Contract"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.Contract = n
		}
	}
	if len(a.Name) == 0 {
		a.Name = catalog.Localized{"en": strings.TrimSuffix(e.ID, ".desktop")}
	}
	return a
}

func localized(e apps.Entry, key string) catalog.Localized {
	out := catalog.Localized{}
	for _, lang := range i18n.Codes {
		if v := e.Fields[key+"["+lang+"]"]; v != "" {
			out[lang] = v
		}
	}
	if v := e.Fields[key]; v != "" {
		if _, ok := out["en"]; !ok {
			out["en"] = v
		}
	}
	return out
}

func localizedKeywords(e apps.Entry) map[string][]string {
	out := map[string][]string{}
	for _, lang := range i18n.Codes {
		if raw := e.Fields["Keywords["+lang+"]"]; raw != "" {
			out[lang] = splitKeywords(raw)
		}
	}
	if raw := e.Fields["Keywords"]; raw != "" {
		if _, ok := out["en"]; !ok {
			out["en"] = splitKeywords(raw)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitKeywords(raw string) []string {
	var out []string
	for _, k := range strings.Split(raw, ";") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}
