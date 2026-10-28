// Command mdsh-gui opens a markdown file in a window and runs its code
// blocks as you pick them, like mdsh -i does in a terminal.
//
//	mdsh-gui [file.md [args...]]
//
// With no file, the window has a button to open one.
package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/run"
)

//go:embed frontend
var frontend embed.FS

func main() {
	if err := mdshGUI(os.Args[1:]); err != nil {
		log.Fatal("mdsh-gui: ", err)
	}
}

func mdshGUI(args []string) error {
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		return err
	}
	a := &App{answers: make(chan run.Answer)}
	app := application.New(application.Options{
		Name:        "mdsh",
		Description: "Run the code blocks in a markdown file",
		Services:    []application.Service{application.NewService(a)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	a.app = app

	a.win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "mdsh",
		Width:  900,
		Height: 800,
		URL:    "/",
	})
	if len(args) > 0 {
		if err := a.open(args[0], args[1:]); err != nil {
			return err
		}
	}
	defer a.close()
	return app.Run()
}

// App is the service the frontend calls.
type App struct {
	app     *application.App
	win     *application.WebviewWindow
	answers chan run.Answer

	mu sync.Mutex
	n  *notebook.Notebook // nil until a file is open
}

// errNoFile is returned by calls that need a file before one is open.
var errNoFile = errors.New("no file is open")

func (a *App) notebook() (*notebook.Notebook, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.n == nil {
		return nil, errNoFile
	}
	return a.n, nil
}

// open opens file, closing any file already open.
func (a *App) open(file string, args []string) error {
	n, err := notebook.Open(file, notebook.Options{
		Args:    args,
		OnEvent: func(e notebook.Event) { a.app.Event.Emit("block", e) },
		Confirm: a.confirm,
	})
	if err != nil {
		return err
	}
	a.close()
	a.mu.Lock()
	a.n = n
	a.mu.Unlock()
	a.win.SetTitle(filepath.Base(n.File()) + " — mdsh")
	return nil
}

func (a *App) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.n != nil {
		a.n.Close()
		a.n = nil
	}
}

// Open asks for a markdown file and opens it. It returns the file's
// path, or "" if none was picked.
func (a *App) Open() (string, error) {
	file, err := a.app.Dialog.OpenFile().
		SetTitle("Open a markdown file").
		AddFilter("Markdown", "*.md;*.markdown").
		AttachToWindow(a.win).
		PromptForSingleSelection()
	if err != nil || file == "" {
		return "", err
	}
	return file, a.open(file, nil)
}

// File is the open document's path, or "" if none is open.
func (a *App) File() string {
	n, err := a.notebook()
	if err != nil {
		return ""
	}
	return n.File()
}

// Cells is the document laid out for drawing.
func (a *App) Cells() ([]notebook.Cell, error) {
	n, err := a.notebook()
	if err != nil {
		return nil, err
	}
	return n.Cells(), nil
}

// RunBlock runs block i alone.
func (a *App) RunBlock(i int) error {
	n, err := a.notebook()
	if err != nil {
		return err
	}
	return n.RunBlock(i)
}

// RunFrom runs block i and the blocks after it.
func (a *App) RunFrom(i int) error {
	n, err := a.notebook()
	if err != nil {
		return err
	}
	return n.RunFrom(i)
}

// Stop stops the running block, answering stop if a block is waiting
// to be confirmed.
func (a *App) Stop() {
	select {
	case a.answers <- run.Stop:
	default:
	}
	if n, err := a.notebook(); err == nil {
		n.Stop()
	}
}

// Save writes the output into the file.
func (a *App) Save() error {
	n, err := a.notebook()
	if err != nil {
		return err
	}
	return n.Save()
}

// Answer replies to the question a confirm event asked: "run", "skip"
// or "stop".
func (a *App) Answer(reply string) error {
	ans, ok := map[string]run.Answer{"run": run.Go, "skip": run.Skip, "stop": run.Stop}[reply]
	if !ok {
		return fmt.Errorf("unknown answer %q", reply)
	}
	select {
	case a.answers <- ans:
		return nil
	default:
		return errors.New("nothing is waiting for an answer")
	}
}

// confirm asks the frontend, from the run's goroutine, and waits for
// Answer.
func (a *App) confirm(block int, question string) run.Answer {
	a.app.Event.Emit("confirm", map[string]any{"block": block, "question": question})
	return <-a.answers
}
