package notebook

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/gdey/markdownshell/doc"
)

// TextboxCell is a textbox, as the frontend draws it.
type TextboxCell struct {
	// Block is the textbox's index in the document, for SetTextbox.
	Block       int    `json:"block"`
	ID          string `json:"id"`
	Text        string `json:"text"`
	Lines       int    `json:"lines"`
	Placeholder string `json:"placeholder,omitempty"`
	// Problem is why its var= binds nothing, if it doesn't.
	Problem string `json:"problem,omitempty"`
}

// ChoiceCell is a choice, as the frontend draws it.
type ChoiceCell struct {
	// Block is the choice's index in the document, for SetChoice.
	Block       int          `json:"block"`
	ID          string       `json:"id"`
	Mode        string       `json:"mode"` // dropdown, radio or multi
	Options     []doc.Option `json:"options"`
	Placeholder string       `json:"placeholder,omitempty"`
	// Problem is why its var= binds nothing, if it doesn't.
	Problem string `json:"problem,omitempty"`
}

// taskMarks are the offsets in the source of the [ ] of every task list
// item in the prose and in blocks' output, such as out=markdown's, in
// order: the n-th is task n for SetTask. Code is neither, so its [ ]s
// don't count.
func (n *Notebook) taskMarks() []int {
	d := n.d
	var marks []int
	add := func(from, to int) {
		if from < to {
			for _, p := range n.marksIn(d.Src[from:to]) {
				marks = append(marks, from+p)
			}
		}
	}
	pos := max(d.FrontMatter.End, len(d.Shebang))
	for _, b := range d.Blocks {
		add(pos, b.Span.Start)
		add(b.Output.Start, b.Output.End)
		pos = max(pos, b.Output.End)
	}
	add(pos, len(d.Src))
	return marks
}

// marksIn are the offsets in md of the [ ] of its task list items and
// table cell checkboxes, in order.
func (n *Notebook) marksIn(md []byte) []int {
	var marks []int
	root := n.md.Parser().Parse(text.NewReader(md))
	_ = ast.Walk(root, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := c.(*east.TaskCheckBox); !ok || !entering {
			return ast.WalkContinue, nil
		}
		if at, ok := c.AttributeString(tableTaskAttr); ok {
			if isTask(md, at.(int)) {
				marks = append(marks, at.(int))
			}
		} else if lines := c.Parent().Lines(); lines.Len() > 0 {
			if p := lines.At(0).Start; isTask(md, p) {
				marks = append(marks, p)
			}
		}
		return ast.WalkSkipChildren, nil
	})
	return marks
}

// isTask reports whether src holds a task's [ ], [x] or [X] at p.
func isTask(src []byte, p int) bool {
	return p+2 < len(src) && src[p] == '[' && src[p+2] == ']' && strings.IndexByte(" xX", src[p+1]) >= 0
}

// SetTask checks task n, as numbered by taskMarks, or unchecks it.
func (n *Notebook) SetTask(task int, done bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	marks := n.taskMarks()
	if task < 0 || task >= len(marks) {
		return fmt.Errorf("no task %d", task)
	}
	mark := byte(' ')
	if done {
		mark = 'x'
	}
	p := marks[task] + 1
	if (n.d.Src[p] != ' ') == done {
		return nil
	}
	n.setResultTask(marks, task, mark)
	src := append([]byte(nil), n.d.Src...)
	src[p] = mark
	return n.setSource(src)
}

// setResultTask sets the mark of task in the output of a block that ran
// this session, if the task is in its output, so that saving, which
// writes that output, keeps it.
func (n *Notebook) setResultTask(marks []int, task int, mark byte) {
	for i, b := range n.d.Blocks {
		r := n.results[i]
		if r == nil || !r.Ran || marks[task] < b.Output.Start || marks[task] >= b.Output.End {
			continue
		}
		j := tasksIn(marks[:task], b.Output) // the task's place in the output
		if out := n.marksIn(r.Stdout); len(j) < len(out) {
			r.Stdout = append([]byte(nil), r.Stdout...)
			r.Stdout[out[len(j)]+1] = mark
		}
		return
	}
}

// SetTextbox replaces the text of textbox i.
func (n *Notebook) SetTextbox(i int, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) || !n.d.Blocks[i].IsTextbox() {
		return fmt.Errorf("block %d isn't a textbox", i)
	}
	return n.setSource(n.d.SetBody(n.d.Blocks[i], text))
}

// SetChoice checks the options of choice i as checked says, one bool
// per option.
func (n *Notebook) SetChoice(i int, checked []bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) || !n.d.Blocks[i].IsChoice() {
		return fmt.Errorf("block %d isn't a choice", i)
	}
	return n.setSource(n.d.SetChoice(n.d.Blocks[i], checked))
}

// tableTaskAttr holds, on a checkbox tableTasks made, the offset of its
// [ ] in what was parsed, since a table cell isn't a list item's line.
const tableTaskAttr = "mdsh-task"

// tableTaskTransformer makes a table cell that starts with [ ] or [x],
// alone or after a list marker as in - [ ], start with a checkbox, as a
// task list item does. GFM has no checkboxes in tables, so GitHub shows
// these as text.
func tableTaskTransformer() parser.Option {
	return parser.WithASTTransformers(util.Prioritized(tableTasks{}, 500))
}

type tableTasks struct{}

var cellTask = regexp.MustCompile(`^(?:[-*+]\s+)?\[([ xX])\](?:\s+|$)`)

func (tableTasks) Transform(root *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var cells []ast.Node
	_ = ast.Walk(root, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := c.(*east.TableCell); ok && entering && c.Lines().Len() > 0 {
			cells = append(cells, c)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	for _, cell := range cells {
		seg := cell.Lines().At(0)
		m := cellTask.FindSubmatchIndex(seg.Value(src))
		if m == nil {
			continue
		}
		// Drop the marker from the cell's text, which inline parsing
		// split into pieces.
		end := seg.Start + m[1]
		for c := cell.FirstChild(); c != nil; {
			next := c.NextSibling()
			t, ok := c.(*ast.Text)
			if !ok || t.Segment.Start >= end {
				break
			}
			if t.Segment.Stop <= end {
				cell.RemoveChild(cell, t)
			} else {
				t.Segment = t.Segment.WithStart(end)
			}
			c = next
		}
		box := east.NewTaskCheckBox(m[2] < m[3] && seg.Value(src)[m[2]] != ' ')
		box.SetAttributeString(tableTaskAttr, seg.Start+m[2]-1)
		if first := cell.FirstChild(); first != nil {
			cell.InsertBefore(cell, first, box)
		} else {
			cell.AppendChild(cell, box)
		}
	}
}
