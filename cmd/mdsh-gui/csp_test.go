package main

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	cspMeta      = regexp.MustCompile(`<meta http-equiv="Content-Security-Policy" content="([^"]*)">`)
	inlineScript = regexp.MustCompile(`(?s)<script(\s[^>]*)?>(.*?)</script>`)
)

// Every page carries a Content-Security-Policy that loads nothing from
// the network, so an image or link in a document can't reach a server
// when it is drawn, and lets in its own inline scripts by hash.
func TestPagesCSP(t *testing.T) {
	pages, err := fs.Glob(frontend, "frontend/*.html")
	if err != nil || len(pages) < 3 {
		t.Fatalf("pages: %v %v", pages, err)
	}
	for _, page := range pages {
		b, err := fs.ReadFile(frontend, page)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		m := cspMeta.FindStringSubmatch(src)
		if m == nil {
			t.Errorf("%s: no Content-Security-Policy meta tag", page)
			continue
		}
		if strings.Index(src, m[0]) > strings.Index(src, "<script") || strings.Index(src, m[0]) > strings.Index(src, "<link") {
			t.Errorf("%s: the policy must come before scripts and styles", page)
		}
		policy := map[string][]string{}
		for _, d := range strings.Split(m[1], ";") {
			if f := strings.Fields(d); len(f) > 0 {
				policy[f[0]] = f[1:]
			}
		}
		if got := policy["default-src"]; !slices.Equal(got, []string{"'self'"}) {
			t.Errorf("%s: default-src %v", page, got)
		}
		for name, sources := range policy {
			for _, s := range sources {
				if s == "*" || strings.Contains(s, ":") && !slices.Contains([]string{"data:", "blob:"}, s) {
					t.Errorf("%s: %s allows %s", page, name, s)
				}
				if (s == "data:" || s == "blob:") && !strings.HasPrefix(name, "img-") && !strings.HasPrefix(name, "media-") && !strings.HasPrefix(name, "font-") {
					t.Errorf("%s: %s allows %s", page, name, s)
				}
			}
		}
		for _, k := range []string{"'unsafe-inline'", "'unsafe-eval'"} {
			if slices.Contains(policy["script-src"], k) {
				t.Errorf("%s: script-src allows %s", page, k)
			}
		}
		for _, s := range inlineScript.FindAllStringSubmatch(src, -1) {
			if strings.Contains(s[1], "src=") {
				continue
			}
			sum := sha256.Sum256([]byte(s[2]))
			hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
			if !slices.Contains(policy["script-src"], hash) {
				t.Errorf("%s: inline script %q needs %s in script-src", page, s[2], hash)
			}
		}
	}
}
