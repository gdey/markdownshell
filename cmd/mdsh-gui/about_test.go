package main

import (
	"runtime/debug"
	"testing"
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
