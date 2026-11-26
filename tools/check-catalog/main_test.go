package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFindsMissingDesktop(t *testing.T) {
	dir := t.TempDir()
	apps := filepath.Join(dir, "apps")
	if err := os.MkdirAll(filepath.Join(apps, "cpz-app-one"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(apps, "cpz-app-two"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(apps, "cpz-app-one", "cpz-one.desktop"),
		filepath.Join(apps, "cpz-app-two", "cpz-two.desktop"),
	} {
		if err := os.WriteFile(p, []byte("[Desktop Entry]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	catalog := filepath.Join(dir, "apps.json")
	if err := os.WriteFile(catalog, []byte(`{"api":1,"apps":[{"id":"cpz-one.desktop"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	missing, err := check(catalog, apps)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || !strings.Contains(missing[0], "cpz-two.desktop") {
		t.Fatalf("missing = %#v", missing)
	}
}
