package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps.json")
	body := `{
	  "version": 1,
	  "generated": "2026-11-26T10:00:00Z",
	  "contract": 1,
	  "apps": [
	    {"id":"p.desktop","package":"cpz-app-p","version":"1.0",
	     "name":{"en":"P","uk":"П"},"comment":{"en":"C"},
	     "categories":["Utility"],"terminal":true,"system":false,"size":10,"contract":1}
	  ]
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Version != 1 || len(idx.Apps) != 1 {
		t.Fatalf("index = %+v", idx)
	}
	if got := idx.Apps[0].Name.Pick("uk"); got != "П" {
		t.Errorf("name = %q", got)
	}
	if idx.Generated.IsZero() {
		t.Error("generated not parsed")
	}
}

func TestMissingIsEmpty(t *testing.T) {
	idx, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if idx.Version != 1 || len(idx.Apps) != 0 {
		t.Fatalf("index = %+v", idx)
	}
}

func TestNewerSchemaIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"apps":[{"id":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Apps) != 0 {
		t.Fatalf("version 2 was not ignored: %+v", idx)
	}
}
