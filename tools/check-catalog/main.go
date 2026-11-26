// Command check-catalog verifies that every app desktop entry in apps/ made it
// into the generated catalog. The release is not signed or published unless
// this passes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	catalogPath := flag.String("catalog", "packaging/cpz-apps-catalog/apps.json", "generated catalog")
	appsDir := flag.String("apps", "apps", "application source directory")
	flag.Parse()

	missing, err := check(*catalogPath, *appsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-catalog:", err)
		os.Exit(1)
	}
	if len(missing) > 0 {
		fmt.Fprintln(os.Stderr, "check-catalog: desktop entries missing from the catalog:")
		for _, id := range missing {
			fmt.Fprintln(os.Stderr, "  "+id)
		}
		os.Exit(1)
	}
	fmt.Printf("check-catalog: %s contains every entry under %s\n", *catalogPath, *appsDir)
}

type index struct {
	Apps []struct {
		ID string `json:"id"`
	} `json:"apps"`
}

func check(catalogPath, appsDir string) ([]string, error) {
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, err
	}
	var idx index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, app := range idx.Apps {
		have[app.ID] = true
	}

	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		desktops, err := filepath.Glob(filepath.Join(appsDir, de.Name(), "*.desktop"))
		if err != nil {
			return nil, err
		}
		if len(desktops) == 0 {
			missing = append(missing, de.Name()+" (no .desktop entry)")
			continue
		}
		for _, p := range desktops {
			id := filepath.Base(p)
			if !have[id] {
				missing = append(missing, id)
			}
		}
	}
	sort.Strings(missing)
	return missing, nil
}
