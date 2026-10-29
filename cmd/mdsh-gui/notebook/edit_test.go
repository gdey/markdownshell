package notebook

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestSetBlock(t *testing.T) {
	file := write(t, src)
	var mu sync.Mutex
	var out strings.Builder
	n, err := Open(file, Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == "stdout" {
			out.WriteString(e.Data)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	text, err := n.Source(0)
	if err != nil {
		t.Fatal(err)
	}
	if text != "```sh {#hello}\necho hello\n```\n" {
		t.Fatalf("Source = %q", text)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.SetBlock(0, "```sh {#hello confirm}\necho bonjour\n```\n"); err != nil {
		t.Fatal(err)
	}
	if !n.Dirty() {
		t.Error("an edit should make the notebook dirty")
	}
	c := n.Cells()
	if c[1].Body != "echo bonjour\n" || !c[1].Ran || c[1].Attrs != "confirm" {
		t.Errorf("after the edit, cell = %+v; the old output should stay until it runs", c[1])
	}
	if err := n.SetBlock(0, "```sh {#hello}\necho bonjour\n```\n"); err != nil {
		t.Fatal(err)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if got := out.String(); got != "hello\nbonjour\n" {
		t.Errorf("output = %q", got)
	}
	mu.Unlock()
	if err := n.SetBlock(0, "```sh\nno fence end\n"); err == nil {
		t.Error("an unclosed block should be refused")
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	if !strings.Contains(string(saved), "```sh {#hello}\necho bonjour\n```\n\n```output {for=hello exit=0}\nbonjour\n```") {
		t.Errorf("saved file:\n%s", saved)
	}
}

func TestAddBlock(t *testing.T) {
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	i, err := n.AddBlock("```sh {#added timeout=5s}\necho added\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if i != 3 {
		t.Errorf("index = %d", i)
	}
	if err := n.RunBlock(i); err != nil {
		t.Fatal(err)
	}
	c := n.Cells()
	last := c[len(c)-1]
	if last.ID != "added" || !last.Ran || !strings.Contains(last.Output, "added") {
		t.Errorf("last cell = %+v", last)
	}
	if got := n.Names().IDs; strings.Join(got, " ") != "hello block-2 fail added" {
		t.Errorf("ids = %v", got)
	}
	for _, bad := range []string{"no fence", "```sh\n```\n", "```sh\na\n```\n\n```sh\nb\n```\n"} {
		if _, err := n.AddBlock(bad); err == nil {
			t.Errorf("AddBlock(%q) should fail", bad)
		}
	}
	if len(n.Cells()) != len(c) {
		t.Error("a failed AddBlock changed the document")
	}
}

func TestCheckInfo(t *testing.T) {
	for _, c := range []struct {
		info string
		want string // "from-to severity" of each problem
	}{
		{`sh {#a timeout=5m confirm="Go?"}`, ""},
		{`sh {#a timeout=5 out=html}`, "7-16 error, 17-25 error"},
		{`sh {#a confrim}`, "7-14 warning"},
		{`go {<include file=x.go from=a}`, "23-29 error"},
		{`go {<include #x file=x.go tty}`, "13-15 error, 26-29 error"},
		{`sh {file=x}`, "4-10 warning"},
		{`sh {run={{$X}`, "4-12 error"},
	} {
		var got []string
		for _, p := range CheckInfo(c.info) {
			got = append(got, strings.Join([]string{strconv.Itoa(p.From) + "-" + strconv.Itoa(p.To), p.Severity}, " "))
			if c.info[p.From:p.To] == "" {
				t.Errorf("%s: empty range", c.info)
			}
		}
		if g := strings.Join(got, ", "); g != c.want {
			t.Errorf("CheckInfo(%s) = %s, want %s", c.info, g, c.want)
		}
	}
}

func TestEditInfo(t *testing.T) {
	info := `sh {#a x=1}`
	info = EditInfo(info, "confirm", "Sure?", false, false)
	info = EditInfo(info, "skip", "", true, false)
	info = EditInfo(info, "x", "", false, true)
	info = SetLang(info, "zsh")
	if info != `zsh {#a confirm=Sure? skip}` {
		t.Errorf("info = %s", info)
	}
}
