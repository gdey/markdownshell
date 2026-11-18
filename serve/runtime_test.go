package serve

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The pages import parts of the Wails runtime by name, and a module that
// lacks one fails to load at all, so runtime.js must export every name
// they import.
func TestRuntimeExports(t *testing.T) {
	imports := regexp.MustCompile(`import\s*\{([^}]*)\}\s*from\s*"/wails/runtime\.js"`)
	files, err := filepath.Glob("../web/src/*.ts")
	if err != nil || len(files) == 0 {
		t.Fatalf("no frontend sources found: %v", err)
	}
	want := map[string]string{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range imports.FindAllStringSubmatch(string(src), -1) {
			for _, name := range strings.Split(m[1], ",") {
				name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "type "))
				if name, _, _ = strings.Cut(name, " as "); name != "" {
					want[name] = filepath.Base(f)
				}
			}
		}
	}
	if len(want) == 0 {
		t.Fatal("found no imports from /wails/runtime.js; is the pattern right?")
	}
	for name, file := range want {
		if !regexp.MustCompile(`export (const|class|function) ` + name + `\b`).Match(runtimeJS) {
			t.Errorf("%s imports %s from /wails/runtime.js, which runtime.js doesn't export", file, name)
		}
	}
}
