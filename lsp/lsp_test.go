package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const book = "# Setup\n\n```sh {#a}\necho a\n```\n\n```textbox {#name var=who}\n```\n\n```sh {#b depends=a,name,zz}\necho b\n```\n\n```choice {#pick var=who}\n- [ ] x\n```\n"

func labels(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}

func TestCheck(t *testing.T) {
	text := book + "```sh {#c timeout=soon bogus}\n```\n"
	var got []string
	for _, d := range Check(text) {
		got = append(got, fmt.Sprintf("%d:%d-%d %s %s", d.Line, d.From, d.To, d.Severity, d.Message))
	}
	want := []string{
		"9:20-24 error name is an input, which never runs, so depends= ignores it",
		"9:25-27 error There is no block zz, so depends= ignores it",
		"13:17-24 error var=who is already bound on line 7, so this one is ignored",
		"16:10-22 error timeout= is a duration such as 30s or 5m",
		"16:23-28 warning mdsh doesn't know bogus and leaves it as it is",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Check:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestComplete(t *testing.T) {
	text := book + "```sh {#c depends=a,}\n```\n```sh {#d sk}\n```\n```sh {#e on_error=}\n```\n```sh {#f run=}\n```\n"
	lines := strings.Split(text, "\n")
	at := func(n int, after string) int {
		t.Helper()
		i := strings.Index(lines[n], after)
		if i < 0 {
			t.Fatalf("no %q on line %d", after, n)
		}
		return i + len(after)
	}
	// depends= offers the blocks above that run, not those listed.
	from, items := Complete(text, "", 16, at(16, "depends=a,"))
	if got := labels(items); !slices.Equal(got, []string{"b"}) {
		t.Errorf("depends= offers %v, want [b]", got)
	}
	if from != at(16, "depends=a,") {
		t.Errorf("depends= replaces from %d", from)
	}
	// Attribute names, from the token's start.
	from, items = Complete(text, "", 18, at(18, "sk"))
	if from != at(18, "{#d ") || !slices.Contains(labels(items), "skip") {
		t.Errorf("names from %d: %v", from, labels(items))
	}
	if slices.Contains(labels(items), "#id") || slices.Contains(labels(items), "lines") {
		t.Errorf("names offer #id, which is there, or an input's: %v", labels(items))
	}
	// A choice's values; a condition's.
	if _, items = Complete(text, "", 20, at(20, "on_error=")); !slices.Equal(labels(items), []string{"stop", "continue"}) {
		t.Errorf("on_error= offers %v", labels(items))
	}
	_, items = Complete(text, "", 22, at(22, "run="))
	if !slices.Contains(labels(items), "once") || !slices.Contains(labels(items), "{{#a/?}}") {
		t.Errorf("run= offers %v", labels(items))
	}
	// A textbox's attributes are its own.
	_, items = Complete(text, "", 6, at(6, "{#name "))
	if got := labels(items); !slices.Contains(got, "lines") || slices.Contains(got, "timeout") {
		t.Errorf("a textbox's names: %v", got)
	}
	// Outside a fence's {…}, nothing.
	if _, items = Complete(text, "", 3, 2); items != nil {
		t.Errorf("in a block's body: %v", labels(items))
	}
}

func TestCompleteFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "doc.md")
	os.WriteFile(file, nil, 0o644)
	os.WriteFile(filepath.Join(dir, "my notes.txt"), nil, 0o644)
	os.Mkdir(file+".assets", 0o755)
	os.WriteFile(filepath.Join(file+".assets", "logo.png"), nil, 0o644)
	text := "```sh {<include file=}\n```\n"
	_, items := Complete(text, file, 0, strings.Index(text, "file=")+5)
	var got []string
	for _, it := range items {
		got = append(got, it.Label+"→"+it.Insert)
	}
	want := []string{"res://logo.png→res://logo.png", `my notes.txt→"my notes.txt"`}
	if !slices.Equal(got, want) {
		t.Errorf("file= offers %v, want %v", got, want)
	}
}

func TestHelp(t *testing.T) {
	spec, from, to, ok := Help(book, 9, 12)
	if !ok || spec.Name != "depends" || from != 10 || to != 28 {
		t.Errorf("Help = %v %d %d %v", spec.Name, from, to, ok)
	}
	if !strings.HasPrefix(HelpText(spec), "**depends** Blocks to run first") {
		t.Errorf("HelpText = %q", HelpText(spec))
	}
	if _, _, _, ok := Help(book, 0, 2); ok {
		t.Error("help on a heading")
	}
}

// client talks to a server the way an editor does.
type client struct {
	t   *testing.T
	in  io.Writer
	out *bufio.Reader
	id  int
}

func (c *client) send(method string, id int, params any) {
	m := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id > 0 {
		m["id"] = id
	}
	b, _ := json.Marshal(m)
	fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(b), b)
}

// next reads the next message.
func (c *client) next() map[string]any {
	c.t.Helper()
	m, err := read(c.out)
	if err != nil {
		c.t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	var out map[string]any
	json.Unmarshal(b, &out)
	// Params and results come back raw: decode them too.
	var full map[string]any
	if m.Params != nil {
		json.Unmarshal(m.Params, &full)
		out["params"] = full
	}
	return out
}

func (c *client) request(method string, params any) any {
	c.t.Helper()
	c.id++
	c.send(method, c.id, params)
	for {
		m, err := readRaw(c.out)
		if err != nil {
			c.t.Fatal(err)
		}
		if id, ok := m["id"].(float64); ok && int(id) == c.id {
			if e, ok := m["error"]; ok {
				c.t.Fatalf("%s: %v", method, e)
			}
			return m["result"]
		}
	}
}

func readRaw(r *bufio.Reader) (map[string]any, error) {
	length := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		fmt.Sscanf(line, "Content-Length: %d", &length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(body, &m)
}

func TestServe(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(inR, outW, ""); outW.Close() }()
	c := &client{t: t, in: inW, out: bufio.NewReader(outR)}

	res := c.request("initialize", map[string]any{"capabilities": map[string]any{}})
	caps := res.(map[string]any)["capabilities"].(map[string]any)
	if caps["positionEncoding"] != "utf-16" || caps["hoverProvider"] != true {
		t.Errorf("capabilities = %v", caps)
	}
	c.send("initialized", 0, map[string]any{})

	// é is two bytes and one UTF-16 unit: columns after it differ.
	text := "# é\n```sh {#a}\n```\n```sh {#b é=1 depends=zz}\n```\n"
	c.send("textDocument/didOpen", 0, map[string]any{"textDocument": map[string]any{"uri": "mdsh://t.md", "languageId": "markdown", "version": 1, "text": text}})
	diag, err := readRaw(c.out)
	if err != nil {
		t.Fatal(err)
	}
	list := diag["params"].(map[string]any)["diagnostics"].([]any)
	var ranges []string
	for _, d := range list {
		r := d.(map[string]any)["range"].(map[string]any)
		s, e := r["start"].(map[string]any), r["end"].(map[string]any)
		ranges = append(ranges, fmt.Sprintf("%v:%v-%v", s["line"], s["character"], e["character"]))
	}
	// é=1 is unknown; zz is no block, after é in UTF-16 units.
	if want := []string{"3:10-13", "3:22-24"}; !slices.Equal(ranges, want) {
		t.Errorf("diagnostic ranges %v, want %v", ranges, want)
	}

	res = c.request("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": "mdsh://t.md"}, "position": map[string]any{"line": 3, "character": 22}})
	items := res.(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["label"] != "a" {
		t.Errorf("completion = %v", items)
	}

	c.send("textDocument/didChange", 0, map[string]any{"textDocument": map[string]any{"uri": "mdsh://t.md", "version": 2}, "contentChanges": []any{map[string]any{"text": "```sh {#a timeout=1m}\n```\n"}}})
	diag, _ = readRaw(c.out)
	if n := len(diag["params"].(map[string]any)["diagnostics"].([]any)); n != 0 {
		t.Errorf("%d diagnostics after the change", n)
	}
	res = c.request("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": "mdsh://t.md"}, "position": map[string]any{"line": 0, "character": 12}})
	if v := res.(map[string]any)["contents"].(map[string]any)["value"].(string); !strings.HasPrefix(v, "**timeout**") {
		t.Errorf("hover = %q", v)
	}

	c.request("shutdown", nil)
	c.send("exit", 0, nil)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
