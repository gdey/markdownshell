package history_test

import (
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/history"
)

func TestOpenBlock(t *testing.T) {
	for _, c := range []struct {
		line, fence, info string
		ok                bool
	}{
		{"# ```bash", "```", "bash", true},
		{"  #```sh {#setup}  ", "```", "sh {#setup}", true},
		{"# ````md", "````", "md", true},
		{"# ~~~ python", "~~~", "python", true},
		{"# ```", "```", "", true},
		{"# ``", "", "", false},
		{"# ```inline```", "", "", false},
		{"# a note", "", "", false},
		{"echo # ```", "", "", false},
	} {
		fence, info, ok := history.OpenBlock(c.line)
		if fence != c.fence || info != c.info || ok != c.ok {
			t.Errorf("OpenBlock(%q) = %q, %q, %v", c.line, fence, info, ok)
		}
	}
}

func TestInBlock(t *testing.T) {
	for in, want := range map[string]bool{
		"# ```bash":                      true,
		"# ```bash\necho hi":             true,
		"# ```bash\necho hi\n```":        false,
		"# ```bash\n```":                 false,
		"# ````md\n```sh\nls\n```":       true, // a shorter fence is body
		"# ````md\n```sh\nls\n```\n````": false,
		"# ~~~sh\n```":                   true, // backticks don't close tildes
		"# ```sh\n    ```":               true, // indented four is body
		"# a note":                       false,
		"echo hi":                        false,
	} {
		if _, got := history.InBlock(in); got != want {
			t.Errorf("InBlock(%q) = %v", in, got)
		}
	}
}

func TestBlock(t *testing.T) {
	for _, c := range []struct {
		in, info, body string
		ok             bool
	}{
		{"# ```bash {#greet}\necho hi\n```", "bash {#greet}", "echo hi\n", true},
		{"# ```sh\na\n\nb\n```  \n", "sh", "a\n\nb\n", true},
		{"# ```sh\n```", "sh", "", true},
		{"# ```sh\necho hi", "", "", false},
		{"# ```sh", "", "", false},
		{"echo hi", "", "", false},
	} {
		info, body, ok := history.Block(c.in)
		if info != c.info || body != c.body || ok != c.ok {
			t.Errorf("Block(%q) = %q, %q, %v", c.in, info, body, ok)
		}
	}
}

func TestWholeBlock(t *testing.T) {
	for in, want := range map[string]bool{
		"# ```bash":                  false,
		"# ```bash\necho 'unclosed":  false,
		"# ```bash\necho `x\n```":    true,
		"# ```bash\nif true; then\n": false,
	} {
		if got := history.Whole("/bin/sh", in); got != want {
			t.Errorf("Whole(%q) = %v", in, got)
		}
	}
}

func TestBlockPrompt(t *testing.T) {
	for info, want := range map[string]string{
		"bash":          "```bash> ",
		"sh {#setup}":   "```sh> ",
		"python id=x":   "```python> ",
		"{#only-an-id}": "```> ",
		"":              "```> ",
	} {
		if got := history.BlockPrompt(info); got != want {
			t.Errorf("BlockPrompt(%q) = %q, want %q", info, got, want)
		}
	}
}

func TestAddBlock(t *testing.T) {
	d, _ := doc.Parse([]byte("# Log\n"))
	b, err := history.AddBlock(d, "bash {#greet}", "echo hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != "greet" || len(d.Blocks) != 1 {
		t.Errorf("block %q, %d blocks", b.ID, len(d.Blocks))
	}
	if want := "# Log\n\n```bash {#greet}\necho hi\n```\n"; string(d.Src) != want {
		t.Errorf("src = %q, want %q", d.Src, want)
	}
	// A body with a fence in it gets a longer one.
	if _, err := history.AddBlock(d, "md", "```sh\nls\n```\n"); err != nil {
		t.Fatal(err)
	}
	if want := "# Log\n\n```bash {#greet}\necho hi\n```\n\n````md\n```sh\nls\n```\n````\n"; string(d.Src) != want {
		t.Errorf("src = %q, want %q", d.Src, want)
	}
	before := string(d.Src)
	if _, err := history.AddBlock(d, "sh", "\n"); err == nil {
		t.Error("added an empty block")
	}
	if string(d.Src) != before {
		t.Errorf("src changed: %q", d.Src)
	}
	// An include's body is filled from its files, so it may be empty.
	b, err = history.AddBlock(d, "markdown {<include file=README.md}", "\n")
	if err != nil {
		t.Fatalf("include: %v", err)
	}
	if !b.IsInclude() || !strings.HasSuffix(string(d.Src), "```markdown {<include file=README.md}\n```\n") {
		t.Errorf("include: src = %q", d.Src)
	}
}
