package looks

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoad(t *testing.T) {
	plain := `{"id": "", "name": "Plain", "order": 0, "since": "1.0", "description": "d", "tagline": "t"}`
	moss := `{"id": "moss", "name": "Moss", "order": 10, "since": "1.6", "description": "d", "tagline": "t", "tokens": "--moss-", "textSize": 110}`
	all, err := Load(fstest.MapFS{
		"plain/look.json": {Data: []byte(plain)},
		"moss/look.json":  {Data: []byte(moss)},
		"moss/look.css":   {Data: []byte(":root[data-look=\"moss\"] {}")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "Plain" || all[1].ID != "moss" || all[1].Dir != "moss" || all[1].TextSize != 110 {
		t.Errorf("Load = %+v", all)
	}
	if !Valid(all, "") || !Valid(all, "moss") || Valid(all, "bogus") {
		t.Error("Valid is wrong")
	}

	for name, tc := range map[string]struct {
		fs   fstest.MapFS
		want string
	}{
		"no css": {fstest.MapFS{
			"plain/look.json": {Data: []byte(plain)},
			"moss/look.json":  {Data: []byte(moss)},
		}, "no look.css"},
		"unknown field": {fstest.MapFS{
			"plain/look.json": {Data: []byte(strings.Replace(plain, `"name"`, `"nmae"`, 1))},
		}, "unknown field"},
		"same id": {fstest.MapFS{
			"plain/look.json": {Data: []byte(plain)},
			"moss/look.json":  {Data: []byte(moss)},
			"moss/look.css":   {Data: []byte("")},
			"moss2/look.json": {Data: []byte(strings.Replace(strings.Replace(moss, `"Moss"`, `"Moss 2"`, 1), `10`, `20`, 1))},
			"moss2/look.css":  {Data: []byte("")},
		}, `same id, "moss"`},
		"bad tokens": {fstest.MapFS{
			"plain/look.json": {Data: []byte(plain)},
			"moss/look.json":  {Data: []byte(strings.Replace(moss, `"--moss-"`, `"moss"`, 1))},
			"moss/look.css":   {Data: []byte("")},
		}, "tokens"},
		"no plain": {fstest.MapFS{
			"moss/look.json": {Data: []byte(moss)},
			"moss/look.css":  {Data: []byte("")},
		}, "Plain"},
	} {
		if _, err := Load(tc.fs); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
