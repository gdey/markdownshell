// Command mdsh-gui opens markdown files in windows and runs their code
// blocks as you pick them, like mdsh -i does in a terminal.
//
//	mdsh-gui [flags] [file.md [args...] | folder]
//	mdsh-gui --version
//
// --version prints the release version, or for a go install or go build
// build the module version or the commit it was built from.
//
// With no file, the window lists the markdown files under the current
// folder to pick from, or under folder when one is given.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

// The frontend is TypeScript in frontend/src, bundled into frontend/bundle.
// The bindings in frontend/src/bindings are generated from App.
//
//go:generate go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0 generate bindings -silent -ts -b -names -i -noindex -noevents -d frontend/src/bindings
//go:generate go run ./internal/build

// build/appicon.svg is the app's icon, for the Settings window's About
// tab.
//
//go:embed frontend/index.html frontend/window.html frontend/settings.html frontend/style.css frontend/teletype.css frontend/bundle frontend/vendor/codemirror.js frontend/vendor/xterm.js frontend/vendor/xterm.css build/appicon.svg
var frontend embed.FS

func main() {
	o, err := parseFlags(os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return
	case err != nil:
		fmt.Fprintln(os.Stderr, "mdsh-gui:", err)
		os.Exit(2)
	case o.version:
		fmt.Println("mdsh-gui", thisBuild())
		return
	}
	if err := mdshGUI(o); err != nil {
		// Not log.Fatal: slog's default logger, which the log package
		// writes to once the app starts, may not reach the terminal.
		fmt.Fprintln(os.Stderr, "mdsh-gui:", err)
		os.Exit(1)
	}
}

// options are what the command line asks for.
type options struct {
	version bool
	// source is the files given to --source, read once, as mdsh does,
	// so a process substitution like <(alias -L) works.
	source string
	// run are the choices for the file named on the command line.
	run notebook.RunOptions
	// args are the file (or folder) and its arguments.
	args []string
}

// parseFlags reads the flags, which mirror mdsh's where they make sense
// for a window. Flags come before the file; what follows the file is
// its arguments.
func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("mdsh-gui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: mdsh-gui [flags] [file.md [args...] | folder]")
		fmt.Fprintln(stderr, "       with no file, the window lists the markdown files under folder (default .)")
		fs.PrintDefaults()
	}
	var sources strings.Builder
	fs.Func("source", "source this file into every shell before blocks run, such as aliases and functions (repeatable; applies to every file opened)", func(p string) error {
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sources.Write(src)
		sources.WriteString("\n")
		return nil
	})
	fs.StringVar(&o.run.Only, "only", "", "run only this block, plus earlier blocks in its env chain")
	fs.BoolVar(&o.run.StopOnError, "stop-on-error", false, "stop at the first failing block")
	fs.BoolVar(&o.run.Step, "step", false, "ask before running each block, as if every block were marked confirm")
	fs.BoolVar(&o.version, "version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.source, o.args = sources.String(), fs.Args()
	if len(o.args) > 0 {
		o.run.Args = o.args[1:]
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for _, name := range []string{"only", "stop-on-error", "step"} {
		if set[name] && len(o.args) == 0 {
			return o, fmt.Errorf("--%s needs a file", name)
		}
	}
	return o, nil
}

func mdshGUI(o options) error {
	args := o.args
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		return err
	}
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
		if o.run.Only != "" || o.run.Step || o.run.StopOnError {
			return errors.New("--only, --step and --stop-on-error need a file, not a folder")
		}
		args = nil
	}
	cwd, _ := os.Getwd()
	// What Wails and the app log goes to the Console pane, as well as
	// where Wails would send it.
	cons := newConsole()
	logger := slog.New(newConsoleHandler(application.DefaultLogger(slog.LevelInfo).Handler(), cons))
	slog.SetDefault(logger)
	a := &App{console: cons, settings: store, cwd: cwd, dir: dir, source: o.source, windows: map[uint]*docWindow{}, tabs: map[int]*docTab{}, tabWindow: map[*docTab]*docWindow{}}
	app := application.New(application.Options{
		Name:        "mdsh",
		Description: "Run the code blocks in a markdown file",
		Services:    []application.Service{application.NewService(a)},
		Assets:      application.AssetOptions{Handler: a.assets(application.AssetFileServerFS(assets)), Middleware: tabCalls},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		ShouldQuit: a.shouldQuit,
		Logger:     logger,
	})
	a.app = app
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.started.Store(true)
		go cons.send(app.Context(), func(e ConsoleEntry) { app.Event.Emit("console", e) })
	})
	a.menu = a.buildMenu()
	a.buildSourceMenu()
	a.buildLinkMenu()
	if runtime.GOOS == "darwin" {
		app.Menu.Set(a.menu)
	}

	t := a.newTab()
	if len(args) > 0 {
		ro := a.runOptions()
		ro.Args, ro.Only, ro.Step, ro.StopOnError = o.run.Args, o.run.Only, o.run.Step, o.run.StopOnError
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
