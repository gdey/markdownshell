package notebook

import (
	"errors"
	"fmt"
	"strings"
	"testing"
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
		"- ```sh {#n}\n  true\n  ```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var got []string
	for _, c := range n.Cells() {
		got = append(got, fmt.Sprintf("%d-%d:%v", c.Line, c.LastLine, c.Movable))
	}
	if want := "1-6:true 4-0:false 8-9:true 11-0:false"; strings.Join(got, " ") != want {
		t.Errorf("cells %q, want %q", strings.Join(got, " "), want)
	}
}
