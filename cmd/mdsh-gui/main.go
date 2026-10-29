// Command mdsh-gui opens markdown files in windows and runs their code
// blocks as you pick them, like mdsh -i does in a terminal.
//
//	mdsh-gui [file.md [args...] | folder]
//	mdsh-gui --version
//
// With no file, the window lists the markdown files under the current
// folder to pick from, or under folder when one is given.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/web"
)

// The frontend is the web package: TypeScript in web/src, bundled into
// web/bundle. The bindings in web/src/bindings are generated from App.
//
//go:generate go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0 generate bindings -silent -ts -b -names -i -noindex -noevents -d ../../web/src/bindings
//go:generate go run ./internal/build

// version is set by release builds.
var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("mdsh-gui", version)
		return
	}
	if err := mdshGUI(os.Args[1:]); err != nil {
		log.Fatal("mdsh-gui: ", err)
	}
}

func mdshGUI(args []string) error {
	path, err := settings.DefaultPath()
	if err != nil {
		return err
	}
	store, err := settings.Open(path)
	if err != nil {
		return err
	}
	dir, err := argDir(args)
	if err != nil {
		return err
	}
	if dir != "" {
		args = nil
	}
	cwd, _ := os.Getwd()
	a := &App{settings: store, cwd: cwd, dir: dir, windows: map[uint]*docWindow{}, tabs: map[int]*docTab{}, tabWindow: map[*docTab]*docWindow{}}
	app := application.New(application.Options{
		Name:        "mdsh",
		Description: "Run the code blocks in a markdown file",
		Services:    []application.Service{application.NewService(a)},
		Assets:      application.AssetOptions{Handler: a.assets(application.AssetFileServerFS(web.FS)), Middleware: tabCalls},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		ShouldQuit: a.shouldQuit,
	})
	a.app = app
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { a.started.Store(true) })
	a.menu = a.buildMenu()
	a.buildSourceMenu()
	a.buildLinkMenu()
	if runtime.GOOS == "darwin" {
		app.Menu.Set(a.menu)
	}

	t := a.newTab()
	if len(args) > 0 {
		ro := notebook.RunOptions{Args: args[1:], Shell: store.Get().Shell}
		if err := t.open(args[0], ro); err != nil {
			return err
		}
	}
	a.newWindow(t)
	return app.Run()
}

// argDir returns the folder named on the command line, made absolute, or
// "" if the first argument is not a folder.
func argDir(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if fi, err := os.Stat(args[0]); err != nil || !fi.IsDir() {
		return "", nil
	}
	if len(args) > 1 {
		return "", errors.New("arguments after a folder are only for a file")
	}
	return filepath.Abs(args[0])
}
