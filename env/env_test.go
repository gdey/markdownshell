package env

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestFilter(t *testing.T) {
	vars := map[string]string{"PATH": "/bin", "HOME": "/h", "USER": "u"}
	tests := []struct {
		name  string
		share []string
		want  map[string]string
	}{
		{"nil keeps all", nil, vars},
		{"empty keeps none", []string{}, map[string]string{}},
		{"names", []string{"PATH", "MISSING"}, map[string]string{"PATH": "/bin"}},
		{"star", []string{"*"}, vars},
		{"ellipsis", []string{"..."}, vars},
		{"ellipsis minus", []string{"...", "-USER"}, map[string]string{"PATH": "/bin", "HOME": "/h"}},
		{"minus before ellipsis", []string{"-USER", "..."}, map[string]string{"PATH": "/bin", "HOME": "/h"}},
		{"minus wins over name", []string{"USER", "PATH", "-USER"}, map[string]string{"PATH": "/bin"}},
		{"minus alone keeps none", []string{"-USER"}, map[string]string{}},
		{"minus missing", []string{"...", "-NOPE"}, vars},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Filter(vars, tt.share); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Filter(%q) = %v, want %v", tt.share, got, tt.want)
			}
		})
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
