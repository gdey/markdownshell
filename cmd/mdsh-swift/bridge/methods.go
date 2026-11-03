package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/session"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/collection"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/library"
	"github.com/gdey/markdownshell/run"
)

// The params of the methods. A method on a document takes its id as
// "doc".
type (
	docP   struct{ Doc int }
	blockP struct {
		Doc   int
		Block int
	}
	fileP struct{ File string }
)

// methods are what the app can call, by name. They follow mdsh-gui's
// App methods, minus the window, dialog and tab bookkeeping the Swift
// app does itself.
func (b *bridge) methods() map[string]handler {
	// onDoc makes a method on an open document.
	onDoc := func(f func(d *document, p docP) (any, error)) handler {
		return method(func(_ context.Context, p docP) (any, error) {
			d, err := b.doc(p.Doc)
			if err != nil {
				return nil, err
			}
			return f(d, p)
		})
	}
	m := map[string]handler{
		// The cast player, which needs no document: the link says
		// where the recording is.
		"castLoad": method(func(_ context.Context, p struct{ Src string }) (any, error) {
			return b.casts.load(p.Src)
		}),
		"castFrame": method(func(_ context.Context, p struct {
			ID    string
			Frame int
		}) (any, error) {
			return b.casts.frame(p.ID, p.Frame)
		}),
		"castGIF": method(func(_ context.Context, p struct{ ID string }) (any, error) {
			return b.casts.gif(p.ID)
		}),
		// The app.
		"hello": method(func(_ context.Context, _ struct{}) (any, error) {
			return map[string]any{"version": 1, "settingsFile": b.settings.Path()}, nil
		}),
		"about": method(func(_ context.Context, _ struct{}) (any, error) { return b.about(), nil }),
		"startScreen": method(func(_ context.Context, p struct{ Dir string }) (any, error) {
			return b.startScreen(p.Dir)
		}),
		"pin": method(func(_ context.Context, p struct {
			File string
			Pin  bool
		}) (any, error) {
			if !filepath.IsAbs(p.File) {
				return nil, fmt.Errorf("pin %q: not an absolute path", p.File)
			}
			return nil, b.settings.Update(func(s *settings.Settings) { s.Pin(filepath.Clean(p.File), p.Pin) })
		}),
		"settings": method(func(_ context.Context, _ struct{}) (any, error) { return b.settings.Get(), nil }),
		"setSettings": method(func(_ context.Context, s settings.Settings) (any, error) {
			if err := b.settings.Update(func(cur *settings.Settings) { edited(cur, s) }); err != nil {
				return nil, err
			}
			got := b.settings.Get()
			b.c.send(event{Event: "settings", Data: got})
			return got, nil
		}),
		"setLastFolder": method(func(_ context.Context, p struct{ Dir string }) (any, error) {
			return nil, b.settings.Update(func(s *settings.Settings) { s.LastFolder = p.Dir })
		}),
		"templates": method(func(_ context.Context, _ struct{}) (any, error) {
			dir := b.settings.Get().TemplateFolder
			if dir == "" {
				return []library.File{}, nil
			}
			l, err := library.List(dir)
			return l.Files, err
		}),
		"createFile": method(func(_ context.Context, p struct{ File, Template string }) (any, error) {
			return b.createFile(p.File, p.Template)
		}),
		"newSession": method(func(_ context.Context, _ struct{}) (any, error) {
			file, _, err := session.Today(b.sessionDir(), b.settings.Get().SessionTemplate, session.LoginShell(os.Getenv), time.Now())
			if err != nil {
				return nil, fmt.Errorf("new session: %w", err)
			}
			return file, nil
		}),
		"lastSession": method(func(_ context.Context, _ struct{}) (any, error) {
			file, err := session.Last(b.sessionDir())
			if err != nil {
				return nil, fmt.Errorf("last session: %w", err)
			}
			if file == "" {
				return nil, fmt.Errorf("no sessions yet: New Session makes one")
			}
			return file, nil
		}),
		"folderSettings": method(func(_ context.Context, p struct{ Rel string }) (any, error) {
			abs, err := b.folder(p.Rel)
			if err != nil {
				return nil, err
			}
			s, err := collection.Read(abs)
			if err != nil {
				return nil, err
			}
			return map[string]any{"path": abs, "settings": s, "images": collection.Images(abs, 200), "writable": collection.Writable(abs)}, nil
		}),
		"setFolderSettings": method(func(_ context.Context, p struct {
			Rel      string
			Settings collection.Settings
		}) (any, error) {
			abs, err := b.folder(p.Rel)
			if err != nil {
				return nil, err
			}
			if !collection.Writable(abs) {
				return nil, errors.New(abs + ": read only")
			}
			return nil, collection.Write(abs, p.Settings)
		}),
		"zshWrapper": method(func(_ context.Context, _ struct{}) (any, error) { return zshWrapperState() }),
		"setZshWrapper": method(func(_ context.Context, p struct{ On bool }) (any, error) {
			return setZshWrapper(p.On)
		}),
		"blockAttrs": method(func(_ context.Context, _ struct{}) (any, error) { return doc.BlockAttrs, nil }),
		"checkInfo":  method(func(_ context.Context, p struct{ Info string }) (any, error) { return notebook.CheckInfo(p.Info), nil }),
		"sourceBlocks": method(func(_ context.Context, p struct{ Text string }) (any, error) {
			return notebook.SourceBlocks(p.Text), nil
		}),

		// Opening and closing documents.
		"open": method(func(_ context.Context, p fileP) (any, error) {
			id, err := b.open(p.File)
			if err != nil {
				return nil, err
			}
			d, _ := b.doc(id)
			return d.info(), nil
		}),
		"openSession": method(func(_ context.Context, p fileP) (any, error) {
			id, err := b.open(p.File)
			if err != nil {
				return nil, err
			}
			d, _ := b.doc(id)
			d.mu.Lock()
			d.openHistory = true
			d.mu.Unlock()
			return d.info(), nil
		}),
		"close": method(func(_ context.Context, p docP) (any, error) { return nil, b.close(p.Doc) }),

		// A document.
		"info":  onDoc(func(d *document, _ docP) (any, error) { return d.info(), nil }),
		"cells": onDoc(func(d *document, _ docP) (any, error) { return d.nb().Cells(), nil }),
		"text":  onDoc(func(d *document, _ docP) (any, error) { return d.nb().Text(), nil }),
		"names": onDoc(func(d *document, _ docP) (any, error) { return d.nb().Names(), nil }),
		"runAll": onDoc(func(d *document, _ docP) (any, error) {
			return nil, d.runWith(func() error { return d.nb().RunFrom(0) })
		}),
		"stop": onDoc(func(d *document, _ docP) (any, error) {
			d.reply(run.Stop)
			d.nb().Stop()
			return nil, nil
		}),
		"save": onDoc(func(d *document, _ docP) (any, error) {
			if err := d.nb().Save(); err != nil {
				return nil, err
			}
			d.saved()
			return d.info(), nil
		}),
		"reload":       onDoc(func(d *document, _ docP) (any, error) { return nil, d.reopen() }),
		"clearOutputs": onDoc(func(d *document, _ docP) (any, error) { return nil, d.nb().ClearOutputs() }),
		"restart":      onDoc(func(d *document, _ docP) (any, error) { return nil, d.nb().Restart() }),
		"undo":         onDoc(func(d *document, _ docP) (any, error) { return d.nb().Undo() }),
		"redo":         onDoc(func(d *document, _ docP) (any, error) { return d.nb().Redo() }),
		"frontMatter":  onDoc(func(d *document, _ docP) (any, error) { return d.nb().FrontMatter() }),
		"reportStart": onDoc(func(d *document, _ docP) (any, error) {
			ran, total := d.nb().Ran()
			file := d.nb().File()
			home, _ := os.UserHomeDir()
			return map[string]any{"ran": ran, "total": total, "dest": strings.TrimSuffix(file, filepath.Ext(file)), "home": home}, nil
		}),
	}

	// Methods on a document with more params. Each P has a Doc.
	add := func(name string, h handler) { m[name] = h }
	add("runBlock", method(func(_ context.Context, p blockP) (any, error) {
		return b.onDocRun(p.Doc, func(n *notebook.Notebook) error { return n.RunBlock(p.Block) })
	}))
	add("runFrom", method(func(_ context.Context, p blockP) (any, error) {
		return b.onDocRun(p.Doc, func(n *notebook.Notebook) error { return n.RunFrom(p.Block) })
	}))
	add("runSection", method(func(_ context.Context, p struct{ Doc, From, To int }) (any, error) {
		return b.onDocRun(p.Doc, func(n *notebook.Notebook) error { return n.RunRange(p.From, p.To) })
	}))
	add("historyEnter", method(func(_ context.Context, p struct {
		Doc   int
		Input string
	}) (any, error) {
		return b.onDocRun(p.Doc, func(n *notebook.Notebook) error { return n.Enter(p.Input) })
	}))
	add("answer", method(func(_ context.Context, p struct {
		Doc   int
		Reply string
	}) (any, error) {
		d, err := b.doc(p.Doc)
		if err != nil {
			return nil, err
		}
		a, ok := answers[p.Reply]
		if !ok {
			return nil, fmt.Errorf("unknown answer %q", p.Reply)
		}
		if !d.reply(a) {
			return nil, fmt.Errorf("nothing is waiting for an answer")
		}
		return nil, nil
	}))
	add("saveAs", method(func(_ context.Context, p struct {
		Doc  int
		File string
	}) (any, error) {
		d, err := b.doc(p.Doc)
		if err != nil {
			return nil, err
		}
		if err := d.nb().SaveAs(p.File); err != nil {
			return nil, err
		}
		d.saved()
		b.opened(d.nb().File())
		return d.info(), nil
	}))
	add("setRunOptions", method(func(_ context.Context, p struct {
		Doc               int
		Args, Only        string
		Step, StopOnError bool
	}) (any, error) {
		d, err := b.doc(p.Doc)
		if err != nil {
			return nil, err
		}
		list, err := run.SplitArgs(p.Args)
		if err != nil {
			return nil, fmt.Errorf("arguments: %w", err)
		}
		ro := d.nb().RunOptions()
		ro.Args, ro.Only, ro.Step, ro.StopOnError = list, p.Only, p.Step, p.StopOnError
		return nil, d.nb().SetRunOptions(ro)
	}))
	add("setHistory", method(func(_ context.Context, p struct {
		Doc int
		On  bool
	}) (any, error) {
		d, err := b.doc(p.Doc)
		if err != nil {
			return nil, err
		}
		d.nb().SetHistoryRC(b.settings.Get().HistoryRC)
		return d.nb().SetHistory(p.On)
	}))
	// historyPrompt is the History prompt at the pane's size, cols by
	// rows, when the app knows it, so a prompt drawn to fit (a rule, a
	// right-aligned part) fits the pane.
	add("historyPrompt", docMethod(b, func(d *document, p struct{ Cols, Rows int }) (any, error) {
		if p.Cols > 0 && p.Rows > 0 {
			return d.nb().PromptAt(p.Cols, p.Rows)
		}
		return d.nb().Prompt()
	}))
	add("historyComplete", docMethod(b, func(d *document, p struct{ Input string }) (any, error) {
		return d.nb().Complete(p.Input), nil
	}))
	add("historyCompletions", docMethod(b, func(d *document, p struct{ Before string }) (any, error) {
		return d.nb().Completions(p.Before)
	}))
	add("setText", docMethod(b, func(d *document, p struct{ Base, Text string }) (any, error) {
		return d.nb().SetText(p.Base, p.Text)
	}))
	add("setTask", docMethod(b, func(d *document, p struct {
		Task int
		Done bool
	}) (any, error) {
		return nil, d.nb().SetTask(p.Task, p.Done)
	}))
	add("setTextbox", docMethod(b, func(d *document, p struct {
		Block int
		Text  string
	}) (any, error) {
		return nil, d.nb().SetTextbox(p.Block, p.Text)
	}))
	add("setChoice", docMethod(b, func(d *document, p struct {
		Block   int
		Checked []bool
	}) (any, error) {
		return nil, d.nb().SetChoice(p.Block, p.Checked)
	}))
	add("setFolds", docMethod(b, func(d *document, p struct{ Folds []notebook.Fold }) (any, error) {
		return nil, d.nb().SetFolds(p.Folds)
	}))
	add("setDepends", docMethod(b, func(d *document, p struct {
		Block int
		Deps  []int
	}) (any, error) {
		return nil, d.nb().SetDepends(p.Block, p.Deps)
	}))
	add("setFrontMatter", docMethod(b, func(d *document, p struct{ FrontMatter notebook.FrontMatter }) (any, error) {
		return nil, d.nb().SetFrontMatter(p.FrontMatter)
	}))
	add("moveBlock", docMethod(b, func(d *document, p struct {
		Block, Dir int
		ByBlock    bool
	}) (any, error) {
		return d.nb().MoveBlock(p.Block, p.Dir, p.ByBlock)
	}))
	add("planMove", docMethod(b, func(d *document, p struct {
		Block, Dir int
		ByBlock    bool
	}) (any, error) {
		return d.nb().PlanMove(p.Block, p.Dir, p.ByBlock)
	}))
	add("moveTo", docMethod(b, func(d *document, p struct{ From, At int }) (any, error) {
		return d.nb().MoveTo(p.From, p.At)
	}))
	// describeFix says what fixWarning would do for warning, a console
	// entry's message; fixWarning makes it, and Undo takes it back.
	// The cast player (cast.go). setCastFrame writes frame=, where
	// the player was left: on block, a tty block, or with block < 0
	// on the cast fence holding body.
	add("setCastFrame", docMethod(b, func(d *document, p struct {
		Block int
		Body  string
		Frame int
	}) (any, error) {
		return nil, d.nb().SetCastFrame(p.Block, p.Body, p.Frame)
	}))
	add("describeFix", docMethod(b, func(d *document, p struct{ Warning string }) (any, error) {
		return d.nb().DescribeFix(p.Warning)
	}))
	add("fixWarning", docMethod(b, func(d *document, p struct{ Warning string }) (any, error) {
		return nil, d.nb().FixWarning(p.Warning)
	}))
	add("deleteBlock", docMethod(b, func(d *document, p struct{ Block int }) (any, error) {
		return d.nb().DeleteBlock(p.Block)
	}))
	add("clearOutput", docMethod(b, func(d *document, p struct{ Block int }) (any, error) {
		return nil, d.nb().ClearOutput(p.Block)
	}))
	add("blockAt", docMethod(b, func(d *document, p struct{ Line int }) (any, error) {
		return d.nb().BlockAt(p.Line), nil
	}))
	add("type", docMethod(b, func(d *document, p struct{ Keys string }) (any, error) {
		d.nb().Type(p.Keys)
		return nil, nil
	}))
	add("setTerminalSize", docMethod(b, func(d *document, p struct{ Cols, Rows int }) (any, error) {
		d.nb().SetTerminalSize(p.Cols, p.Rows)
		return nil, nil
	}))
	add("reportCommand", docMethod(b, func(d *document, p struct{ Report notebook.Report }) (any, error) {
		home, _ := os.UserHomeDir()
		return d.nb().ReportCommand(p.Report, home), nil
	}))
	add("createReport", docMethod(b, func(d *document, p struct{ Report notebook.Report }) (any, error) {
		return nil, d.nb().WriteReport(p.Report)
	}))
	return m
}

// docMethod makes a method on an open document, named by the "doc"
// param, that takes params P besides it.
func docMethod[P any](b *bridge, f func(d *document, p P) (any, error)) handler {
	id := method(func(_ context.Context, p docP) (any, error) { return p.Doc, nil })
	rest := method(func(_ context.Context, p P) (any, error) { return p, nil })
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		v, err := id(ctx, raw)
		if err != nil {
			return nil, err
		}
		d, err := b.doc(v.(int))
		if err != nil {
			return nil, err
		}
		pv, err := rest(ctx, raw)
		if err != nil {
			return nil, err
		}
		return f(d, pv.(P))
	}
}

// onDocRun runs f on document id's notebook as a run: autosave's
// read-only check first, a "ran" event and autosave after.
func (b *bridge) onDocRun(id int, f func(n *notebook.Notebook) error) (any, error) {
	d, err := b.doc(id)
	if err != nil {
		return nil, err
	}
	return nil, d.runWith(func() error { return f(d.nb()) })
}

func (d *document) runWith(f func() error) error {
	if err := d.beforeRun(); err != nil {
		return err
	}
	err := d.run(f)
	d.afterRun()
	return err
}
