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
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/lsp"
	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/ui"
	"github.com/gdey/markdownshell/web"
)

// The frontend is the web package: TypeScript in web/src, bundled into
// web/bundle. The bindings in web/src/bindings are generated from ui.App.
//
//go:generate go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0 generate bindings -silent -ts -b -names -i -noindex -noevents -d ../../web/src/bindings
//go:generate go run ./internal/build

// build/appicon.svg is the app's icon, for the Settings window's About
// tab.
//
//go:embed build/appicon.svg
var appicon embed.FS

func main() {
	// --lsp serves the language server on stdin and stdout, for the
	// Neovim source pane: mdsh-gui runs itself, so it needs no mdsh on
	// the PATH. A file after it is the document, for completing paths.
	if len(os.Args) > 1 && os.Args[1] == "--lsp" {
		file := ""
		if len(os.Args) > 2 {
			file = os.Args[2]
		}
		if err := lsp.Serve(os.Stdin, os.Stdout, file); err != nil {
			fmt.Fprintln(os.Stderr, "mdsh-gui --lsp:", err)
			os.Exit(1)
		}
		return
	}
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
	g := &gui{cli: newCLIHost()}
	a := ui.New(g, store, cwd, dir, o.source)
	g.ctrl = ui.ControlOf(a)
	g.ctrl.SetBuild(thisBuild())
	// The Neovim pane's language server is this program, run with --lsp.
	if exe, err := os.Executable(); err == nil {
		g.ctrl.SetLSP(func(file string) []string { return []string{exe, "--lsp", file} })
	}
	// What Wails and the app log goes to the Console pane, as well as
	// where Wails would send it.
	cons := g.ctrl.Console()
	logger := slog.New(ui.NewConsoleHandler(application.DefaultLogger(slog.LevelInfo).Handler(), cons))
	slog.SetDefault(logger)
	// A link left by a moved app is pointed back here, and an
	// AppImage's copy brought up to this version, quietly.
	go func() {
		if err := g.cli.repair(); err != nil {
			slog.Warn("updating the mdsh command line tool", "err", err)
		}
	}()
	app := application.New(application.Options{
		Name:        "mdsh",
		Description: "Run the code blocks in a markdown file",
		Services:    []application.Service{application.NewService(a)},
		Assets:      application.AssetOptions{Handler: withIcon(g.ctrl.Assets(application.AssetFileServerFS(web.FS))), Middleware: ui.TabCalls},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		ShouldQuit: g.ctrl.ShouldQuit,
		Logger:     logger,
	})
	g.app = app
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		g.started.Store(true)
		go cons.Send(app.Context(), func(e ui.ConsoleEntry) { app.Event.Emit("console", e) }, func(seqs []int) { app.Event.Emit("console-stale", seqs) })
	})
	g.menu = g.buildMenu()
	g.buildSourceMenu()
	g.buildLinkMenu()
	g.buildCastMenu()
	if runtime.GOOS == "darwin" {
		app.Menu.Set(g.menu)
	}

	file, run := "", o.run
	if len(args) > 0 {
		file, run.Args = args[0], args[1:]
	}
	if err := g.ctrl.Start(file, run); err != nil {
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

// withIcon serves the app icon at /appicon.svg, for the About tab, and
// everything else with next.
func withIcon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/appicon.svg" {
			http.ServeFileFS(w, r, appicon, "build/appicon.svg")
			return
		}
		next.ServeHTTP(w, r)
	})
}
