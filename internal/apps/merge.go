package apps

import (
	"sort"
	"strings"

	"github.com/jmas/cpzos-apps/internal/catalog"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// App is the merged view of an installed entry and its catalog record.
type App struct {
	ID         string
	Path       string // desktop entry path, empty for not-installed apps
	Package    string
	Version    string
	Name       string
	Comment    string
	Icon       string
	Categories []string
	Keywords   []string
	Exec       string
	Terminal   bool
	System     bool
	Installed  bool
	Size       int64
}

// Build merges installed entries with the catalog. Installed data wins.
func Build(lang string, installed []Entry, idx *catalog.Index) []App {
	byID := idx.ByID()
	out := make([]App, 0, len(installed)+len(idx.Apps))
	seen := map[string]bool{}

	for _, e := range installed {
		a := App{
			ID:         e.ID,
			Path:       e.Path,
			Name:       e.Name(lang),
			Comment:    e.Comment(lang),
			Icon:       e.Icon(),
			Categories: e.Categories(),
			Keywords:   e.Keywords(lang),
			Exec:       e.Exec(),
			Terminal:   e.Terminal(),
			System:     e.System(),
			Installed:  true,
		}
		if rec, ok := byID[e.ID]; ok {
			a.Package = rec.Package
			a.Version = rec.Version
			a.Size = rec.Size
			a.System = a.System || rec.System
			if len(a.Categories) == 0 {
				a.Categories = rec.Categories
			}
			if a.Name == "" {
				a.Name = rec.Name.Pick(lang)
			}
			if a.Comment == "" {
				a.Comment = rec.Comment.Pick(lang)
			}
		}
		if a.Name == "" {
			a.Name = strings.TrimSuffix(e.ID, ".desktop")
		}
		out = append(out, a)
		seen[e.ID] = true
	}

	for _, rec := range idx.Apps {
		if seen[rec.ID] || rec.Contract > 1 {
			continue
		}
		out = append(out, App{
			ID:         rec.ID,
			Package:    rec.Package,
			Version:    rec.Version,
			Name:       rec.Name.Pick(lang),
			Comment:    rec.Comment.Pick(lang),
			Icon:       rec.Icon,
			Categories: rec.Categories,
			Keywords:   keywordsFor(rec, lang),
			Terminal:   rec.Terminal,
			System:     rec.System,
			Size:       rec.Size,
		})
	}

	sortApps(out, lang)
	return out
}

func keywordsFor(rec catalog.App, lang string) []string {
	var out []string
	out = append(out, rec.Keywords[lang]...)
	if lang != "en" {
		out = append(out, rec.Keywords["en"]...)
	}
	return out
}

func sortApps(list []App, lang string) {
	c := collate.New(language.Make(lang), collate.IgnoreCase)
	sort.SliceStable(list, func(i, j int) bool {
		return c.CompareString(list[i].Name, list[j].Name) < 0
	})
}

var mainCategories = map[string]bool{
	"AudioVideo": true, "Development": true, "Education": true, "Game": true,
	"Graphics": true, "Network": true, "Office": true, "Science": true,
	"Settings": true, "System": true, "Utility": true,
}

// MainCategory returns the first freedesktop main category, else the first.
func MainCategory(cats []string) string {
	for _, c := range cats {
		if mainCategories[c] {
			return c
		}
	}
	for _, c := range cats {
		if c != "" {
			return c
		}
	}
	return ""
}

// Match reports whether an app matches a lower-case query.
func Match(a App, query, lang string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	fields := []string{a.Name, a.Comment, a.Package, a.ID, a.Version}
	fields = append(fields, a.Keywords...)
	fields = append(fields, a.Categories...)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}
