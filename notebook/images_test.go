package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImages(t *testing.T) {
	file := write(t, "![logo](img/logo.svg) and ![web](https://example.com/a.png)\n\n"+
		"```sh {#plot out=image}\necho '<svg xmlns=\"http://www.w3.org/2000/svg\"/>'\n```\n")
	dir := filepath.Dir(file)
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img", "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	prose := n.Cells()[0].HTML
	logo := DocPrefix + filepath.ToSlash(filepath.Join(dir, "img", "logo.svg")) + "?v="
	if !strings.Contains(prose, `src="`+logo) || !strings.Contains(prose, `src="https://example.com/a.png"`) {
		t.Errorf("prose images: %s", prose)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	out := n.Cells()[1].Output
	plot := DocPrefix + filepath.ToSlash(filepath.Join(dir, "doc.md.assets", "plot.svg")) + "?v="
	if !strings.Contains(out, `src="`+plot) {
		t.Errorf("image output: %s", out)
	}
}

func TestBeside(t *testing.T) {
	for link, want := range map[string]bool{
		"a.png": true, "img/a%20b.png": true, "../up.png": true,
		"/abs.png": false, "data:image/png;base64,AA": false, "https://x/a.png": false, "#frag": false,
	} {
		if _, ok := beside("/d", link); ok != want {
			t.Errorf("beside(%q) = %v, want %v", link, ok, want)
		}
	}
}
