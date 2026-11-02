package env

import (
	"bytes"
	"context"
	"io"
	"maps"
	"reflect"
	"strings"
	"testing"
)

func TestFilter(t *testing.T) {
	vars := map[string]string{"PATH": "/bin", "HOME": "/h", "USER": "u", "LC_ALL": "C", "TOKEN": "t", "GREETING": "hi"}
	base := map[string]string{"PATH": "/bin", "HOME": "/h", "USER": "u", "LC_ALL": "C"}
	tests := []struct {
		name   string
		share  []string
		want   map[string]string
		strict map[string]string
	}{
		{"nil keeps all", nil, vars, vars},
		{"empty keeps base", []string{}, base, map[string]string{}},
		{"names", []string{"GREETING", "MISSING"}, with(base, "GREETING", "hi"), map[string]string{"GREETING": "hi"}},
		{"star", []string{"*"}, vars, vars},
		{"ellipsis", []string{"..."}, vars, vars},
		{"ellipsis minus", []string{"...", "-TOKEN"}, without(vars, "TOKEN"), without(vars, "TOKEN")},
		{"minus before ellipsis", []string{"-TOKEN", "..."}, without(vars, "TOKEN"), without(vars, "TOKEN")},
		{"minus wins over name", []string{"GREETING", "PATH", "-GREETING"}, base, map[string]string{"PATH": "/bin"}},
		{"minus drops base", []string{"-USER", "-LC_ALL"}, map[string]string{"PATH": "/bin", "HOME": "/h"}, map[string]string{}},
		{"minus missing", []string{"...", "-NOPE"}, vars, vars},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Filter(vars, tt.share); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Filter(%q) = %v, want %v", tt.share, got, tt.want)
			}
			if got := FilterStrict(vars, tt.share); !reflect.DeepEqual(got, tt.strict) {
				t.Errorf("FilterStrict(%q) = %v, want %v", tt.share, got, tt.strict)
			}
		})
	}
}

func with(m map[string]string, k, v string) map[string]string {
	out := maps.Clone(m)
	out[k] = v
	return out
}

func without(m map[string]string, k string) map[string]string {
	out := maps.Clone(m)
	delete(out, k)
	return out
}

func TestIsBase(t *testing.T) {
	for name, want := range map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
		"TERM": true, "LANG": true, "LC_CTYPE": true, "TMPDIR": true,
		"__CF_USER_TEXT_ENCODING": true,
		"GITHUB_TOKEN":            false, "SSH_AUTH_SOCK": false, "XDG_RUNTIME_DIR": false,
		"PATHX": false, "LC": false,
	} {
		if got := IsBase(name); got != want {
			t.Errorf("IsBase(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestInit(t *testing.T) {
	sh, err := Start(Config{Init: "alias hi='echo hi'\nfalse"})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	var out bytes.Buffer
	if rc, _, err := sh.Run(context.Background(), "hi there", &out, io.Discard); err != nil || rc != 0 || out.String() != "hi there\n" {
		t.Errorf("rc %d, err %v, out %q", rc, err, out.String())
	}

	if _, err := Start(Config{Init: "echo oops >&2; exit 3"}); err == nil || !strings.Contains(err.Error(), "oops") {
		t.Errorf("err = %v, want the Init's stderr", err)
	}
}
