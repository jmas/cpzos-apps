package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// DefaultPath is installed by the cpz-apps-catalog package.
const DefaultPath = "/usr/share/cpz/catalog/apps.json"

// Localized maps a language code to a string.
type Localized map[string]string

// Pick resolves exact language, English, then any value.
func (l Localized) Pick(lang string) string {
	if v := l[lang]; v != "" {
		return v
	}
	if v := l["en"]; v != "" {
		return v
	}
	for _, v := range l {
		if v != "" {
			return v
		}
	}
	return ""
}

// App is one catalog record, generated from a package's desktop entry.
type App struct {
	ID         string              `json:"id"`
	Package    string              `json:"package"`
	Version    string              `json:"version"`
	Name       Localized           `json:"name"`
	Comment    Localized           `json:"comment"`
	Keywords   map[string][]string `json:"keywords,omitempty"`
	Categories []string            `json:"categories"`
	Terminal   bool                `json:"terminal"`
	System     bool                `json:"system"`
	Size       int64               `json:"size"`
	Icon       string              `json:"icon"`
	Contract   int                 `json:"contract"`
}

// Index is the schema-1 catalog.
type Index struct {
	Version   int       `json:"version"`
	Generated time.Time `json:"generated"`
	Contract  int       `json:"contract"`
	Apps      []App     `json:"apps"`
}

// Load reads a catalog. A missing file is an empty catalog, not an error.
func Load(path string) (*Index, error) {
	empty := &Index{Version: 1}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return empty, nil
		}
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if idx.Version != 1 {
		// A newer schema is ignored: Apps Manager falls back to installed apps.
		return empty, nil
	}
	return &idx, nil
}

// ByID indexes the available apps.
func (i *Index) ByID() map[string]App {
	out := make(map[string]App, len(i.Apps))
	for _, a := range i.Apps {
		out[a.ID] = a
	}
	return out
}
