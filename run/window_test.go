package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func TestWindow(t *testing.T) {
	log := "build\nok a\nFAIL b\n  want 1\n  got 2\nok c\nFAIL d\ndone\n"
	tests := []struct {
		name  string
		data  string
		inc   doc.Include
		want  string
		first int
		warn  string
	}{
		{"from to", log, doc.Include{From: 2, To: 3}, "ok a\nFAIL b\n", 2, ""},
		{"match", log, doc.Include{Match: "^FAIL"}, "FAIL b\n  want 1\n  got 2\nok c\nFAIL d\ndone\n", 3, ""},
		{"match lines", log, doc.Include{Match: "^FAIL", Lines: 3}, "FAIL b\n  want 1\n  got 2\n", 3, ""},
		{"match before", log, doc.Include{Match: "^FAIL", Before: 1, Lines: 1}, "ok a\nFAIL b\n", 2, ""},
		{"before clipped", log, doc.Include{Match: "^build", Before: 5, Lines: 1}, "build\n", 1, ""},
		{"lines clipped", log, doc.Include{Match: "^FAIL d", Lines: 9}, "FAIL d\ndone\n", 7, ""},
		{"until", log, doc.Include{Match: "^FAIL", Until: "^ok"}, "FAIL b\n  want 1\n  got 2\nok c\n", 3, ""},
		{"until the start line", log, doc.Include{Match: "^FAIL", Until: "FAIL"}, "FAIL b\n", 3, ""},
		{"until no match runs to the end", log, doc.Include{Match: "^FAIL d", Until: "^nope"}, "FAIL d\ndone\n", 7, ""},
		{"from to before", log, doc.Include{From: 3, To: 4, Before: 1}, "ok a\nFAIL b\n  want 1\n", 2, ""},
		{"to past the end", log, doc.Include{From: 7, To: 20, Before: 1}, "ok c\nFAIL d\ndone\n", 6, ""},
		{"from lines", log, doc.Include{From: 4, Lines: 2}, "  want 1\n  got 2\n", 4, ""},
		{"lines alone", log, doc.Include{Lines: 2}, "build\nok a\n", 1, ""},
		{"no match", log, doc.Include{File: "b.log", Match: "^panic"}, "", 0, `include: no line of b.log matches "^panic"`},
		{"crlf", "a\r\nFAIL x\r\nb\r\n", doc.Include{Match: `x$`, Until: `^b$`}, "FAIL x\r\nb\r\n", 2, ""},
		{"no final newline", "a\nFAIL", doc.Include{Match: "FAIL"}, "FAIL", 2, ""},
	}
	for _, tt := range tests {
		w, err := run.Window([]byte(tt.data), tt.inc, "text")
		if err != nil || string(w.Text) != tt.want || w.First != tt.first || w.Warning != tt.warn {
			t.Errorf("%s: got %q at %d, warning %q, err %v; want %q at %d, warning %q", tt.name, w.Text, w.First, w.Warning, err, tt.want, tt.first, tt.warn)
		}
	}
	if _, err := run.Window([]byte(log), doc.Include{From: 20, Lines: 1}, "text"); err == nil || !strings.Contains(err.Error(), "past the end") {
		t.Errorf("from past the end: err = %v", err)
	}
	if _, err := run.Window([]byte(log), doc.Include{From: 20}, "text"); err == nil {
		t.Errorf("from past the end, no window: err = nil")
	}
}

const goSrc = `package a

import "fmt"

// Size is how big.
type Size int

// T holds things.
type T[K comparable] struct {
	n int
}

// Get gets.
//
// It says so.
func (t *T[K]) Get() int {
	return t.n
}

func (t T[K]) Put() {}

// Get is a function.
func Get() {}

const (
	// A is first.
	A = iota
	B
)

var X, Y = 1, 2

func init() {}

func init() {
	fmt.Println()
}

func (m *M[K, V]) Len() int { return 0 }

func (p (P)) Name() {}

func (s []int) Odd() {}
`

func TestWindowSymbol(t *testing.T) {
	tests := []struct {
		symbol    string
		noComment bool
		lines     int
		want      string
		first     int
		warn      string
	}{
		{"Size", false, 0, "// Size is how big.\ntype Size int\n", 5, ""},
		{"T", true, 0, "type T[K comparable] struct {\n\tn int\n}\n", 9, ""},
		{"T.Get", false, 0, "// Get gets.\n//\n// It says so.\nfunc (t *T[K]) Get() int {\n\treturn t.n\n}\n", 13, ""},
		{"(*T).Get", true, 0, "func (t *T[K]) Get() int {\n\treturn t.n\n}\n", 16, ""},
		{"T.Put", false, 0, "func (t T[K]) Put() {}\n", 20, ""},
		{"Get", false, 0, "// Get is a function.\nfunc Get() {}\n", 22, ""},
		{"A", false, 0, "\t// A is first.\n\tA = iota\n", 26, ""},
		{"B", false, 0, "\tB\n", 28, ""},
		{"Y", false, 0, "var X, Y = 1, 2\n", 31, ""},
		{"T.Get", false, 1, "// Get gets.\n", 13, ""},
		{"init", false, 0, "func init() {}\n", 33, "include: a.go defines init 2 times; showing the first"},
		{"Nope", false, 0, "", 0, "include: a.go defines no Nope"},
		{"Size.Get", false, 0, "", 0, "include: a.go defines no Size.Get"},
		{"M.Len", false, 0, "func (m *M[K, V]) Len() int { return 0 }\n", 39, ""},
		{"P.Name", false, 0, "func (p (P)) Name() {}\n", 41, ""},
		{"Odd", false, 0, "func (s []int) Odd() {}\n", 43, ""},
	}
	for _, tt := range tests {
		inc := doc.Include{File: "a.go", Symbol: tt.symbol, NoComment: tt.noComment, Lines: tt.lines}
		w, err := run.Window([]byte(goSrc), inc, "go")
		if err != nil || string(w.Text) != tt.want || w.First != tt.first || w.Warning != tt.warn {
			t.Errorf("%s: got %q at %d, warning %q, err %v; want %q at %d, warning %q", tt.symbol, w.Text, w.First, w.Warning, err, tt.want, tt.first, tt.warn)
		}
	}
	if w, _ := run.Window([]byte("package a\nfunc {"), doc.Include{File: "a.go", Symbol: "f"}, "go"); w.First != 0 || w.Warning != "include: a.go defines no f" {
		t.Errorf("broken Go: %+v", w)
	}
	broken := "package a\n\nfunc f() {\n\treturn\n}\n\nfunc g( {\n"
	if w, _ := run.Window([]byte(broken), doc.Include{File: "a.go", Symbol: "f"}, "go"); w.First != 3 || string(w.Text) != "func f() {\n\treturn\n}\n" || !strings.Contains(w.Warning, "doesn't read as Go") {
		t.Errorf("broken Go, best guess: %+v", w)
	}
	// Under a plain fence, a .go file is still Go.
	if w, _ := run.Window([]byte(broken), doc.Include{File: "a.go", Symbol: "f"}, "text"); w.First != 3 || !strings.Contains(w.Warning, "doesn't read as Go") {
		t.Errorf("broken Go, plain fence: %+v", w)
	}
	twice := broken + "\nfunc f() {}\n"
	if w, _ := run.Window([]byte(twice), doc.Include{File: "a.go", Symbol: "f"}, "go"); w.First != 3 || !strings.HasSuffix(w.Warning, "best guess; a.go defines f 2 times; showing the first") {
		t.Errorf("broken Go, two: %+v", w)
	}
}

const pySrc = `import os


# greet says hello.
@cache
def greet(name,
          loud=False):
    x = "}"

    return x


class Server:
    """Serves."""

    def handle(self):
        pass

    def stop(self):
        pass


def handle():
    pass
`

const jsSrc = `// greet says hello.
/* It says it
   twice. */
function greet(n) {
  const s = ` + "`${n}}`" + `;
  return s + "{";
}

class Server {
  handle() { return 1 }
}

const arrow = (a) => a + 1;
let late = 2
`

const rsSrc = `use std::fmt;

/// A point.
#[derive(Debug)]
struct Point {
    x: i32,
}

impl Point {
    fn len(&self) -> i32
    {
        0
    }
}

fn greet(n: &str) -> String {
    format!("{}", n)
}

struct Unit;

trait Shape {}
`

const shSrc = `#!/bin/sh

# greet says hello.
greet() {
  echo "}"
}

function other {
  if true; then
    echo hi
  fi
}
`

const rbSrc = `class Foo
  def greet(n)
    n
  end
end
`

func TestWindowSymbolOther(t *testing.T) {
	tests := []struct {
		name, src, file, lang, symbol string
		noComment                     bool
		first, last                   int
		warn                          string
	}{
		{"py func", pySrc, "a.py", "python", "greet", false, 4, 10, ""},
		{"py no comment", pySrc, "a.py", "python", "greet", true, 5, 10, ""},
		{"py class", pySrc, "a.py", "python", "Server", false, 13, 20, ""},
		{"py method", pySrc, "a.py", "python", "Server.stop", false, 19, 20, ""},
		{"py two", pySrc, "a.py", "python", "handle", false, 16, 17, "include: a.py defines handle 2 times; showing the first"},
		{"py by name", pySrc, "a.py", "", "greet", false, 4, 10, ""},
		{"py none", pySrc, "a.py", "python", "os", false, 0, 0, "include: a.py defines no os"},
		{"js func", jsSrc, "a.js", "js", "greet", false, 1, 7, ""},
		{"js class", jsSrc, "a.js", "javascript", "Server", false, 9, 11, ""},
		{"js arrow", jsSrc, "a.js", "js", "arrow", false, 13, 13, ""},
		{"js let", jsSrc, "a.js", "js", "late", false, 14, 14, ""},
		{"rust struct", rsSrc, "a.rs", "rust", "Point", false, 3, 7, ""},
		{"rust method", rsSrc, "a.rs", "rust", "Point.len", false, 10, 13, ""},
		{"rust fn", rsSrc, "a.rs", "rust", "greet", false, 16, 18, ""},
		{"rust unit", rsSrc, "a.rs", "rust", "Unit", false, 20, 20, ""},
		{"rust trait", rsSrc, "a.rs", "rust", "Shape", false, 22, 22, ""},
		{"sh func", shSrc, "a.sh", "sh", "greet", false, 3, 6, ""},
		{"sh function", shSrc, "a.sh", "bash", "other", false, 8, 12, ""},
		{"ruby end", rbSrc, "a.rb", "ruby", "greet", false, 2, 4, ""},
		{"ruby class", rbSrc, "a.rb", "ruby", "Foo", false, 1, 5, ""},
		{"yaml key", "a:\n  b: 1\n\n  c:\n    - x\nd: 2\n", "a.yaml", "yaml", "a", false, 1, 5, ""},
		{"yaml nested", "a:\n  b: 1\n\n  c:\n    - x\nd: 2\n", "a.yaml", "yaml", "a.c", false, 4, 5, ""},
		{"end of file, no body", "fn f()", "a.rs", "rust", "f", false, 1, 1, ""},
		{"unclosed brace", "fn f() {\n    x\n\n", "a.rs", "rust", "f", false, 1, 3, ""},
		{"unclosed head", "def f(\n    a,\n", "a.py", "python", "f", false, 1, 2, ""},
		{"shell brace on a line of its own", "f()\n{\n  echo hi\n}\n", "a.sh", "sh", "f", false, 1, 4, ""},
		{"no lexer", "x\n", "a.zzz", "", "x", false, 0, 0, "include: a.zzz defines no x that symbol= can find: it has no lexer for a.zzz"},
	}
	for _, tt := range tests {
		inc := doc.Include{File: tt.file, Symbol: tt.symbol, NoComment: tt.noComment}
		w, err := run.Window([]byte(tt.src), inc, tt.lang)
		last := 0
		if w.First > 0 {
			last = w.First + strings.Count(strings.TrimSuffix(string(w.Text), "\n"), "\n")
		}
		if err != nil || w.First != tt.first || last != tt.last || w.Warning != tt.warn {
			t.Errorf("%s: got lines %d-%d, warning %q, err %v; want %d-%d, %q\n%s", tt.name, w.First, last, w.Warning, err, tt.first, tt.last, tt.warn, w.Text)
		}
	}
}

// A window numbers its lines from where it starts in the file, and one
// that matches nothing is empty without failing the run.
func TestIncludeWindow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.log"), []byte("one\ntwo\nFAIL three\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "```text {<include file=b.log match=^FAIL before=1 lines=2 number_on}\n```\n\n" +
		"```text {<include file=b.log match=^panic}\nold\n```\n"
	d, results := runDoc(t, src, run.Options{Dir: dir, Now: fixedNow})
	if r := results[0]; r.FirstLine != 2 || string(r.Stdout) != "two\nFAIL three\nfour\n" {
		t.Errorf("window = %q at %d", r.Stdout, r.FirstLine)
	}
	if r := results[1]; r.Failed() || len(r.Stdout) != 0 || !strings.Contains(r.Warning, "no line of b.log matches") {
		t.Errorf("no match = %+v", r)
	}
	printed := string(render.Markdown(d, results, render.Options{HideCode: true, HideOutput: true}))
	if !strings.Contains(printed, "2  two\n3  FAIL three\n4  four\n") {
		t.Errorf("printed:\n%s", printed)
	}
	once := string(render.Markdown(d, results, render.Options{WriteBack: true}))
	want := "```text {<include file=b.log match=^FAIL before=1 lines=2 number_on}\ntwo\nFAIL three\nfour\n```\n\n" +
		"```text {<include file=b.log match=^panic}\n```\n"
	if once != want {
		t.Errorf("write-back:\n%s\nwant\n%s", once, want)
	}
}
