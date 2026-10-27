package env

import (
	"reflect"
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
