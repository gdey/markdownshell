// Command mdsh-gui opens markdown files in windows and runs their code
// blocks as you pick them, like mdsh -i does in a terminal.
//
//	mdsh-gui [file.md [args...] | folder]
//	mdsh-gui --version
//	mdsh-gui --serve=stdio [file.md [args...] | folder]
//
// With no file, the window lists the markdown files under the current
// folder to pick from, or under folder when one is given.
//
// With --serve=stdio it opens no window and serves the notebook on stdin
// and stdout instead, as mdsh --serve=stdio does.
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

	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/ui"
	"github.com/gdey/markdownshell/web"
)

// The frontend is the web package: TypeScript in web/src, bundled into
// web/bundle. The bindings in web/src/bindings are generated from ui.App.
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
	if len(os.Args) >= 2 && os.Args[1] == stdioFlag {
		if err := serveStdio(os.Args[2:]); err != nil {
			log.Fatal("mdsh-gui: ", err)
		}
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
	g := &gui{}
	a := ui.New(g, store, cwd, dir)
	g.ctrl = ui.ControlOf(a)
	app := application.New(application.Options{
		Name:        "mdsh",
		Description: "Run the code blocks in a markdown file",
		Services:    []application.Service{application.NewService(a)},
		Assets:      application.AssetOptions{Handler: g.ctrl.Assets(application.AssetFileServerFS(web.FS)), Middleware: ui.TabCalls},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		ShouldQuit: g.ctrl.ShouldQuit,
	})
	g.app = app
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { g.started.Store(true) })
	g.menu = g.buildMenu()
	g.buildSourceMenu()
	g.buildLinkMenu()
	if runtime.GOOS == "darwin" {
		app.Menu.Set(g.menu)
	}

	file, fileArgs := "", []string(nil)
	if len(args) > 0 {
		file, fileArgs = args[0], args[1:]
	}
	if err := g.ctrl.Start(file, fileArgs); err != nil {
		return err
	}
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
