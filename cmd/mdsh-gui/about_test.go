package main

import (
	"runtime/debug"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestBuildInfo(t *testing.T) {
	vcs := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.time", Value: "2026-10-30T12:00:00Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	installed := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.1"}}
	for _, c := range []struct {
		name    string
		version string
		bi      *debug.BuildInfo
		want    build
		text    string
	}{
		{"release", "1.2.1", nil, build{Version: "1.2.1"}, "1.2.1"},
		{"release with vcs", "1.2.1", vcs, build{"1.2.1", "0123456789ab", "2026-10-30T12:00:00Z", true}, "1.2.1 (0123456789ab modified, 2026-10-30T12:00:00Z)"},
		{"go install", "", installed, build{Version: "v1.2.1"}, "v1.2.1"},
		{"go build", "", vcs, build{"dev", "0123456789ab", "2026-10-30T12:00:00Z", true}, "dev (0123456789ab modified, 2026-10-30T12:00:00Z)"},
		{"no info", "", nil, build{Version: "dev"}, "dev"},
		{"devel without vcs", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, build{Version: "dev"}, "dev"},
	} {
		got := buildInfo(c.version, c.bi)
		if got != c.want {
			t.Errorf("%s: buildInfo = %+v, want %+v", c.name, got, c.want)
		}
		if got.String() != c.text {
			t.Errorf("%s: String = %q, want %q", c.name, got.String(), c.text)
		}
	}
}

func TestToolVersion(t *testing.T) {
	if v := toolVersion("/bin/sh"); v != "" {
		t.Errorf("sh version = %q, want none", v)
	}
	if v := toolVersion("/no/such/program"); v != "" {
		t.Errorf("missing program version = %q, want none", v)
	}
}

// The app menu's About opens the About tab of Settings, not the stock
// Cocoa About panel the About role shows.
func TestAppMenuAbout(t *testing.T) {
	// The roles below About need an application to name.
	application.New(application.Options{Name: "mdsh"})
	a := &App{}
	m := application.NewMenu()
	a.addAppMenu(m)
	app := m.ItemAt(0).GetSubmenu()
	if app.FindByRole(application.About) != nil {
		t.Error("the app menu has the stock About item")
	}
	if app.FindByLabel("About mdsh") == nil {
		t.Error("the app menu has no About mdsh item")
	}
	for tab, want := range map[string]string{"": "/settings.html", "about": "/settings.html#about"} {
		if got := settingsURL(tab); got != want {
			t.Errorf("settingsURL(%q) = %q, want %q", tab, got, want)
		}
	}
}

func TestSettingsTab(t *testing.T) {
	for at, page := range map[string]string{"": "/settings.html", "about": "/settings.html#about"} {
		got := (&docTab{id: 5, page: settingsPage, at: at}).info()
		if got.Title != "Settings" || got.Page != page || got.ReadOnly || got.File != "" || got.URL != "" {
			t.Errorf("at %q: info = %+v", at, got)
		}
	}
}
