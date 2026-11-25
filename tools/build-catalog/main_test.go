package main

import (
	"testing"

	"github.com/jmas/cpzos-apps/internal/apps"
)

func TestParsePKGINFO(t *testing.T) {
	info, err := parsePKGINFO("pkgname = cpz-app-demo\npkgver = 1.2.3-1\nsize = 4096\n")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "cpz-app-demo" || info.Version != "1.2.3-1" || info.Size != 4096 {
		t.Fatalf("info = %+v", info)
	}
	if _, err := parsePKGINFO("pkgver = 1\n"); err == nil {
		t.Fatal("missing pkgname accepted")
	}
}

func TestBuildAppLocalizes(t *testing.T) {
	e := apps.Entry{ID: "demo.desktop", Path: "/usr/share/applications/demo.desktop", Fields: map[string]string{
		"Name": "Demo", "Name[uk]": "Демо",
		"Comment": "Demo app", "Comment[uk]": "Демо додаток",
		"Exec": "demo", "Terminal": "true", "Categories": "Utility;",
		"Keywords": "demo;", "Keywords[uk]": "демо;",
		"X-CPZ-Contract": "1", "X-CPZ-System": "false",
	}}
	a := buildApp(pkgInfo{Name: "cpz-app-demo", Version: "1.2.3-1", Size: 4096}, e)
	if a.ID != "demo.desktop" || a.Package != "cpz-app-demo" {
		t.Fatalf("app = %+v", a)
	}
	if a.Name["en"] != "Demo" || a.Name["uk"] != "Демо" {
		t.Errorf("name = %#v", a.Name)
	}
	if len(a.Keywords["uk"]) != 1 || a.Keywords["uk"][0] != "демо" {
		t.Errorf("keywords = %#v", a.Keywords)
	}
	if !a.Terminal || a.System {
		t.Errorf("flags = %+v", a)
	}
}

func TestSystemPackageFlag(t *testing.T) {
	e := apps.Entry{ID: "x.desktop", Fields: map[string]string{
		"Name": "X", "Exec": "x", "Categories": "System;",
	}}
	a := buildApp(pkgInfo{Name: "cpz-system-apps"}, e)
	if !a.System {
		t.Error("system package not flagged")
	}
}
