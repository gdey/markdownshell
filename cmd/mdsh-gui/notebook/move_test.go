package notebook

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
)

func TestMoveBlock(t *testing.T) {
	src := "# T\n\nIntro.\n\n" +
		"```sh {#a}\necho a\n```\n\n" +
		"Between.\n\n" +
		"```sh {#b run={{#a/?}}==0}\necho b\n```\n\n" +
		"- list\n\n  ```sh {#n}\n  true\n  ```\n\n" +
		"End.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	order := func() string {
		var ids []string
		for _, b := range n.d.Blocks {
			ids = append(ids, b.ID)
		}
		return strings.Join(ids, " ")
	}

	// Down past "Between." only.
	m, err := n.MoveBlock(0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "Between.\n\n```sh {#a}") || m.Block != 0 || m.Line != 7 {
		t.Errorf("after moving a down a cell (block %d, line %d):\n%s", m.Block, m.Line, n.Text())
	}
	if m.Note == "" || len(m.Warnings) != 0 {
		t.Errorf("note %q, warnings %q", m.Note, m.Warnings)
	}
	if !n.results[0].Ran {
		t.Error("a lost its result")
	}
	// Past b, which now reads a block below it.
	m, err = n.MoveBlock(0, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if order() != "b a n" || m.Block != 1 || len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "comes after it") {
		t.Errorf("order %s, block %d, warnings %q", order(), m.Block, m.Warnings)
	}
	if !n.results[1].Ran || n.results[1].Block.ID != "a" {
		t.Error("a's result didn't follow it")
	}
	// Undo puts it back, warning and all.
	if what, err := n.Undo(); err != nil || what != "move block" || order() != "a b n" {
		t.Errorf("undo: %q, %v, order %s", what, err, order())
	}
	if !n.results[0].Ran {
		t.Error("a lost its result on undo")
	}
	// The list's block doesn't move, and nothing moves past the top.
	if _, err := n.MoveBlock(2, -1, false); err == nil {
		t.Error("a nested block moved")
	}
	n.Undo()
	if _, err := n.MoveBlock(0, -1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := n.MoveBlock(0, -1, false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.Text(), "```sh {#a}") {
		t.Errorf("a isn't at the top:\n%s", n.Text())
	}
	if _, err := n.MoveBlock(0, -1, false); !errors.Is(err, ErrNoMove) {
		t.Errorf("past the top: %v", err)
	}

	// PlanMove changes nothing.
	before := n.Text()
	m, err = n.PlanMove(0, 1, true)
	if err != nil || n.Text() != before || m.Base != before || m.Text == before {
		t.Errorf("plan: %v", err)
	}
}

func TestMoveNamesBlocks(t *testing.T) {
	src := "```sh\necho one\n```\n\n" +
		"```sh\necho two\n```\n\n" +
		"It said {{#block-1/?}}.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	m, err := n.MoveBlock(0, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Text, "block-1/") || !strings.Contains(m.Text, "echo two\n```\n\n```sh {#block-") {
		t.Errorf("text:\n%s", m.Text)
	}
}

func TestBlockAt(t *testing.T) {
	src := "Intro.\n\n" + // 1
		"```sh {#a tty}\n" + // 3
		"sh\n" +
		"```\n" +
		"```tape {for=a}\n" + // 6
		"Enter\n" +
		"```\n\n" + // 8
		"- ```sh {#n}\n  true\n  ```\n" // 10
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	for line, want := range map[int]int{1: -1, 2: -1, 3: 0, 5: 0, 7: 0, 8: 0, 9: -1, 10: -1} {
		if got := n.BlockAt(line); got != want {
			t.Errorf("BlockAt(%d) = %d, want %d", line, got, want)
		}
	}
}

func TestCellsMovable(t *testing.T) {
	src := "```sh {#a tty}\nsh\n```\n```tape {for=a}\nEnter\n```\n\n" +
		"```textbox {#t}\n```\n\n" +
		"- ```sh {#n}\n  true\n  ```\n\n" +
		"```go\nfunc main() {}\n```\n\n" +
		"```mermaid\ngraph TD\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var got []string
	for _, c := range n.Cells() {
		got = append(got, fmt.Sprintf("%d-%d:%v", c.Line, c.LastLine, c.Movable))
	}
	if want := "1-6:true 4-0:false 8-9:true 11-0:false 15-17:true 19-21:true"; strings.Join(got, " ") != want {
		t.Errorf("cells %q, want %q", strings.Join(got, " "), want)
	}
}

func TestMoveTo(t *testing.T) {
	src := "# One\n\nA.\n\n```sh\necho a\n```\n\n" +
		"# Two\n\n```sh\necho b\n```\n\nB.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	from := func(text string) int {
		t.Helper()
		for _, c := range n.Cells() {
			if strings.HasPrefix(n.Text()[c.From:], text) {
				if !c.Drop {
					t.Errorf("cell %q has no drop", text)
				}
				return c.From
			}
		}
		t.Fatalf("no cell %q", text)
		return 0
	}
	// Dragging block a to the end names the blocks first, so nothing is
	// renamed, and the offsets still hold.
	m, err := n.MoveTo(from("```sh\necho a"), -1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "B.\n\n```sh {#"+n.d.Blocks[1].ID+"}\necho a\n```\n") || m.Block != 1 || m.Note == "" {
		t.Errorf("block %d, note %q:\n%s", m.Block, m.Note, n.Text())
	}
	// Section Two goes first, and Undo puts it back.
	m, err = n.MoveTo(from("# Two"), from("# One"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.Text(), "# Two\n") || m.Block != -1 || m.Line != 1 || m.FromLine != 5 {
		t.Errorf("line %d from %d:\n%s", m.Line, m.FromLine, n.Text())
	}
	if what, err := n.Undo(); err != nil || what != "move section" || !strings.HasPrefix(n.Text(), "# One\n") {
		t.Errorf("undo %q %v:\n%s", what, err, n.Text())
	}
	// A section can't go into the middle of another.
	if _, err := n.MoveTo(from("# Two"), from("A.")); err == nil {
		t.Error("section moved mid-section")
	}
}

func TestMoveFence(t *testing.T) {
	src := "A.\n\n```go\nfunc main() {}\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if _, err := n.MoveTo(strings.Index(src, "```"), 0); err != nil {
		t.Fatal(err)
	}
	if want := "```go\nfunc main() {}\n```\n\nA.\n"; n.Text() != want {
		t.Errorf("text %q, want %q", n.Text(), want)
	}
}

func TestMoveDependsOrder(t *testing.T) {
	src := "```sh\ntrue\n```\n\n```sh {#a}\ntrue\n```\n\nX.\n\n```sh {#b depends=a}\ntrue\n```\n\nY.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	cell := func(text string) Cell {
		t.Helper()
		for _, c := range n.Cells() {
			if strings.HasPrefix(n.Text()[c.From:], text) {
				return c
			}
		}
		t.Fatalf("no cell %q", text)
		return Cell{}
	}
	// a can go as far down as just above b; b as far up as just below a.
	a, b := cell("```sh {#a"), cell("```sh {#b")
	if a.DropAfter != -1 || a.DropBefore != b.From {
		t.Errorf("a's limits %d, %d; b is at %d", a.DropAfter, a.DropBefore, b.From)
	}
	if b.DropAfter != a.From || b.DropBefore != -1 {
		t.Errorf("b's limits %d, %d; a is at %d", b.DropAfter, b.DropBefore, a.From)
	}
	if c := cell("X."); c.DropAfter != -1 || c.DropBefore != -1 {
		t.Errorf("prose has limits %d, %d", c.DropAfter, c.DropBefore)
	}
	// Past them a drag or a step fails, and changes nothing, not even
	// the unnamed block's id.
	if _, err := n.MoveTo(a.From, -1); !errors.Is(err, doc.ErrDependsOrder) {
		t.Errorf("a to the end: %v", err)
	}
	if _, err := n.MoveTo(b.From, a.From); !errors.Is(err, doc.ErrDependsOrder) {
		t.Errorf("b above a: %v", err)
	}
	if _, err := n.MoveBlock(1, 1, true); !errors.Is(err, doc.ErrDependsOrder) {
		t.Errorf("a down past b: %v", err)
	}
	if n.Text() != src {
		t.Errorf("text changed:\n%s", n.Text())
	}
	// Up to the limit is fine.
	if _, err := n.MoveTo(a.From, b.From); err != nil || !strings.Contains(n.Text(), "X.\n\n```sh {#a}") {
		t.Errorf("a to just above b: %v\n%s", err, n.Text())
	}
}
