package looks

import (
	"strings"
	"testing"
	"testing/fstest"
)

// css is a look.css with front matter fm and rules after it.
func css(fm, rules string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("/* +++\n" + fm + "\n+++\n\n   A look. */\n" + rules)}
}

const (
	plain = `id = ""
name = "Plain"
order = 0
since = "1.0"
description = "d"
tagline = "t"`
	moss = `id = "moss"
name = "Moss"
order = 10
since = "1.6"
description = "d"
tagline = "t"
tokens = "--moss-"
textSize = 110

[texture]
label = "Moss bed"
rules = "Lichen"`
	mossRules = `:root[data-look="moss"] { --bg: #fff; }`
)

func TestLoad(t *testing.T) {
	all, err := Load(fstest.MapFS{
		"plain/look.css": css(plain, ""),
		"moss/look.css":  css(moss, mossRules),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "Plain" || all[1].ID != "moss" || all[1].Dir != "moss" || all[1].TextSize != 110 || all[1].Texture.Rules != "Lichen" {
		t.Errorf("Load = %+v", all)
	}
	if !Valid(all, "") || !Valid(all, "moss") || Valid(all, "bogus") {
		t.Error("Valid is wrong")
	}

	for name, tc := range map[string]struct {
		fs   fstest.MapFS
		want string
	}{
		"no front matter": {fstest.MapFS{
			"plain/look.css": css(plain, ""),
			"moss/look.css":  {Data: []byte("/* Moss. */\n" + mossRules)},
		}, "no front matter"},
		"unknown key": {fstest.MapFS{
			"plain/look.css": css(strings.Replace(plain, "name", "nmae", 1), ""),
		}, "unknown keys"},
		"a string for a number": {fstest.MapFS{
			"plain/look.css": css(plain, ""),
			"moss/look.css":  css(strings.Replace(moss, "order = 10", `order = "10"`, 1), mossRules),
		}, "order"},
		"same id": {fstest.MapFS{
			"plain/look.css": css(plain, ""),
			"moss/look.css":  css(moss, mossRules),
			"moss2/look.css": css(strings.NewReplacer(`"Moss"`, `"Moss 2"`, "order = 10", "order = 20").Replace(moss), mossRules),
		}, `same id, "moss"`},
		"bad tokens": {fstest.MapFS{
			"plain/look.css": css(plain, ""),
			"moss/look.css":  css(strings.Replace(moss, `"--moss-"`, `"moss"`, 1), mossRules),
		}, "tokens"},
		"plain with rules": {fstest.MapFS{
			"plain/look.css": css(plain, ":root { --bg: red; }"),
		}, "has rules"},
		"no plain": {fstest.MapFS{
			"moss/look.css": css(moss, mossRules),
		}, "Plain"},
	} {
		if _, err := Load(tc.fs); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
