package notebook

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
)

const tasksSrc = "---\ntodo: |\n  - [ ] not a task\n---\n" +
	"- [ ] one\n- [x] two\n\n" +
	"<!-- mdsh:if {{$MDSH_GUI_NEVER_SET}} -->\n- [X] hidden\n<!-- mdsh:end -->\n" +
	"> - [ ] quoted\n\n" +
	"```sh {#a out=markdown}\necho '- [ ] output'\n```\n" +
	"<!-- mdsh:output for=\"a\" -->\n- [ ] output\n<!-- /mdsh:output -->\n\n" +
	"```\n- [ ] code\n```\n\n" +
	"1. [ ] last\n"

func TestTasks(t *testing.T) {
	file := write(t, tasksSrc)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var got [][]int
	for _, c := range n.Cells() {
		if len(c.Tasks) > 0 {
			got = append(got, c.Tasks)
		}
		if len(c.OutputTasks) > 0 {
			got = append(got, c.OutputTasks)
		}
		if boxes := strings.Count(c.HTML, `type="checkbox"`); c.Block < 0 && boxes != len(c.Tasks) {
			t.Errorf("cell %q has %d checkboxes and tasks %v", tasksSrc[c.From:c.To], boxes, c.Tasks)
		}
	}
	// The hidden task is 2, so no cell shows it. Task 4 is in a's
	// output.
	if want := [][]int{{0, 1}, {3}, {4}, {5}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("tasks = %v, want %v", got, want)
	}

	for _, s := range []struct {
		task int
		done bool
	}{{0, true}, {1, false}, {2, false}, {3, true}, {4, true}, {5, true}, {5, true}} {
		if err := n.SetTask(s.task, s.done); err != nil {
			t.Fatal(err)
		}
	}
	if err := n.SetTask(6, true); err == nil {
		t.Error("SetTask(6) made no error")
	}
	want := strings.NewReplacer("- [ ] one", "- [x] one", "- [x] two", "- [ ] two", "- [X] hidden", "- [ ] hidden", "> - [ ] quoted", "> - [x] quoted", "-->\n- [ ] output", "-->\n- [x] output", "1. [ ] last", "1. [x] last").Replace(tasksSrc)
	if got := n.Text(); got != want {
		t.Errorf("text = %q\nwant %q", got, want)
	}
	if !n.Dirty() {
		t.Error("not dirty after SetTask")
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if saved, _ := os.ReadFile(file); string(saved) != want {
		t.Errorf("saved %q", saved)
	}
}

func TestTextbox(t *testing.T) {
	src := "Host:\n\n```textbox {#host placeholder=\"Which host?\"}\n```\n\n" +
		"- notes:\n  ````textbox {#notes lines=3}\n  old\n  ````\n\n" +
		"```sh {#show}\necho \"$MDSH_MODE ${MDSH_TEXT_host-unset}\"\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var boxes []TextboxCell
	for _, c := range n.Cells() {
		if c.Textbox != nil {
			boxes = append(boxes, *c.Textbox)
		}
	}
	want := []TextboxCell{
		{Block: 0, ID: "host", Text: "", Lines: 1, Placeholder: "Which host?"},
		{Block: 1, ID: "notes", Text: "old", Lines: 3},
	}
	if !slices.Equal(boxes, want) {
		t.Fatalf("textboxes = %+v, want %+v", boxes, want)
	}

	if err := n.SetTextbox(0, "db1"); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTextbox(1, "one\n\n```\nthree"); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTextbox(2, "x"); err == nil {
		t.Error("SetTextbox on an sh block made no error")
	}
	wantSrc := "Host:\n\n```textbox {#host placeholder=\"Which host?\"}\ndb1\n```\n\n" +
		"- notes:\n  ````textbox {#notes lines=3}\n  one\n\n  ```\n  three\n  ````\n\n" +
		"```sh {#show}\necho \"$MDSH_MODE ${MDSH_TEXT_host-unset}\"\n```\n"
	if got := n.Text(); got != wantSrc {
		t.Errorf("text = %q\nwant %q", got, wantSrc)
	}
	for _, c := range n.Cells() {
		if c.Textbox != nil && c.Textbox.ID == "notes" && c.Textbox.Text != "one\n\n```\nthree" {
			t.Errorf("notes = %q", c.Textbox.Text)
		}
	}

	if err := n.RunBlock(2); err != nil {
		t.Fatal(err)
	}
	if got := string(n.results[2].Stdout); got != "interactive unset\n" {
		t.Errorf("show printed %q", got)
	}
	if err := n.SetTextbox(0, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.Text(), "Host:\n\n```textbox {#host placeholder=\"Which host?\"}\n```\n") {
		t.Errorf("emptied: %q", n.Text())
	}
}

func TestChoice(t *testing.T) {
	src := "```choice {#env mode=radio var=env}\n- [ ] staging\n- [x] prod\nnot an option\n```\n\n" +
		"> ```choice {#regions mode=multi}\n> - [X] us\n> - [ ] eu\n> ```\n\n" +
		"<!-- mdsh:if {{$MDSH_VAR_env}}==staging -->\nStaging only.\n<!-- mdsh:end -->\n\n" +
		"Deploying to {{$MDSH_VAR_env}}.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	html := func() (s string, choices []ChoiceCell) {
		for _, c := range n.Cells() {
			s += c.HTML
			if c.Choice != nil {
				choices = append(choices, *c.Choice)
			}
		}
		return s, choices
	}
	s, choices := html()
	if len(choices) != 2 || choices[0].Mode != "radio" || choices[1].Mode != "multi" ||
		!slices.Equal(choices[0].Options, []doc.Option{{Label: "staging"}, {Label: "prod", Checked: true}}) {
		t.Fatalf("choices = %+v", choices)
	}
	if strings.Contains(s, "Staging only.") || !strings.Contains(s, "Deploying to prod.") {
		t.Errorf("before: %s", s)
	}

	if err := n.SetChoice(0, []bool{true, false}); err != nil {
		t.Fatal(err)
	}
	if err := n.SetChoice(1, []bool{true, true}); err != nil {
		t.Fatal(err)
	}
	if err := n.SetChoice(2, nil); err == nil {
		t.Error("SetChoice on a non-choice made no error")
	}
	want := strings.NewReplacer("- [ ] staging\n- [x] prod", "- [x] staging\n- [ ] prod", "> - [ ] eu", "> - [x] eu").Replace(src)
	if got := n.Text(); got != want {
		t.Errorf("text = %q\nwant %q", got, want)
	}
	if s, _ := html(); !strings.Contains(s, "Staging only.") || !strings.Contains(s, "Deploying to staging.") {
		t.Errorf("after: %s", s)
	}
}

func TestVarTaken(t *testing.T) {
	src := "```textbox {#a var=env}\none\n```\n\n```choice {#b var=env}\n- [x] two\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var problems []string
	for _, c := range n.Cells() {
		switch {
		case c.Textbox != nil:
			problems = append(problems, c.Textbox.Problem)
		case c.Choice != nil:
			problems = append(problems, c.Choice.Problem)
		}
	}
	if want := []string{"", `var=env is already bound by "a" on line 1, so this one is ignored`}; !slices.Equal(problems, want) {
		t.Errorf("problems = %q, want %q", problems, want)
	}
}

func TestTableTasks(t *testing.T) {
	src := "- [ ] before\n\n| Item | Title |\n|------|-------|\n| - [ ]  | one |\n| [x] done | two |\n| [ ]x | not a task |\n| `[ ]` | code |\n\n" +
		"```sh {#t out=markdown}\necho '| a |'; echo '|---|'; echo '| [ ] out |'\n```\n" +
		"<!-- mdsh:output for=\"t\" -->\n| a |\n|---|\n| [ ] out |\n<!-- /mdsh:output -->\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var tasks, outTasks []int
	var html string
	for _, c := range n.Cells() {
		tasks = append(tasks, c.Tasks...)
		outTasks = append(outTasks, c.OutputTasks...)
		html += c.HTML + c.Output
	}
	if !slices.Equal(tasks, []int{0, 1, 2}) || !slices.Equal(outTasks, []int{3}) {
		t.Errorf("tasks = %v, output tasks %v", tasks, outTasks)
	}
	for _, want := range []string{`<td><input disabled="" type="checkbox"> </td>`, `<td><input checked="" disabled="" type="checkbox"> done</td>`, `<td>[ ]x</td>`, `<td><code>[ ]</code></td>`} {
		if !strings.Contains(html, want) {
			t.Errorf("no %s in\n%s", want, html)
		}
	}
	// Output tables get checkboxes too.
	if strings.Count(html, `type="checkbox"`) != 4 {
		t.Errorf("checkboxes: %s", html)
	}
	if err := n.SetTask(1, true); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTask(2, false); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTask(3, true); err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer("| - [ ]  | one |", "| - [x]  | one |", "| [x] done |", "| [ ] done |", "|---|\n| [ ] out |", "|---|\n| [x] out |").Replace(src)
	if got := n.Text(); got != want {
		t.Errorf("text = %q\nwant %q", got, want)
	}
}

// TestOutputTaskSaved checks that checking a task in the output of a
// block that ran this session survives saving, which writes that output.
func TestOutputTaskSaved(t *testing.T) {
	src := "```sh {#t out=markdown}\necho '- [ ] a'; echo '- [ ] b'\n```\n"
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTask(1, true); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	if want := src + "\n<!-- mdsh:output for=\"t\" -->\n- [ ] a\n- [x] b\n<!-- /mdsh:output -->\n"; string(saved) != want {
		t.Errorf("saved %q\nwant %q", saved, want)
	}
}

// TestRunOnce checks that Run all leaves a run=once block with saved
// output alone and Run on the block runs it.
func TestRunOnce(t *testing.T) {
	src := "```sh {#a run=once}\necho new\n```\n\n```output {for=a}\nold\n```\n"
	file := write(t, src)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunFrom(0); err != nil {
		t.Fatal(err)
	}
	if c := n.Cells()[0]; !strings.Contains(c.Output, "old") {
		t.Errorf("after Run all, output = %q", c.Output)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if saved, _ := os.ReadFile(file); string(saved) != src {
		t.Errorf("after Run all: %q", saved)
	}
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	if saved, _ := os.ReadFile(file); !strings.Contains(string(saved), "new\n```\n") {
		t.Errorf("after Run: %q", saved)
	}
}
