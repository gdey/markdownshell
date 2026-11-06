package main

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
)

// TestFrontend bundles each of the frontend's tests in frontend/test, as
// the app is bundled, and runs it with node, which fails it by exiting
// with an error.
func TestFrontend(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node isn't on the PATH")
	}
	tests, err := filepath.Glob("../../frontend/test/*.test.ts")
	if err != nil || len(tests) == 0 {
		t.Fatal("no frontend tests", err)
	}
	vendor, err := filepath.Abs("../../frontend/vendor/codemirror.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(filepath.Base(test), func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "test.mjs")
			res := api.Build(api.BuildOptions{
				EntryPoints: []string{test},
				Outfile:     out,
				Bundle:      true,
				Format:      api.FormatESModule,
				Platform:    api.PlatformNode,
				Write:       true,
				LogLevel:    api.LogLevelError,
				Plugins: []api.Plugin{{
					Name: "vendored",
					Setup: func(b api.PluginBuild) {
						b.OnResolve(api.OnResolveOptions{Filter: vendored}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
							return api.OnResolveResult{Path: vendor}, nil
						})
					},
				}},
			})
			if len(res.Errors) > 0 {
				t.Fatal(res.Errors)
			}
			if b, err := exec.Command(node, out).CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
		})
	}
}
