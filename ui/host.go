// Package ui is the notebook the frontend in web drives: tabs, windows
// and the calls their pages make, apart from the program that shows the
// windows.
package ui

import "context"

// Host is what an App needs from the program that shows its windows:
// mdsh-gui's Wails windows, say.
// Each window's page is web's window.html.
type Host interface {
	// NewWindow opens a window showing the tab bar. The host tells the
	// App's Control when it is closing, gets files dropped on it or
	// comes to the front.
	NewWindow() Window
	// WindowOf is the window a frontend call came from.
	WindowOf(ctx context.Context) (id uint, ok bool)
	// Current is the window in front, if the host knows.
	Current() (id uint, ok bool)
	// EmitAll sends an event to every window.
	EmitAll(name string, data any)
	// OnMain runs fn where the host may close windows.
	OnMain(fn func())

	// ShowError tells the user about err, over win if it isn't nil.
	ShowError(win Window, err error)
	// Ask asks a question over win with buttons and returns at once;
	// the button picked runs its Do, if it has one.
	Ask(win Window, q Question)
	// OpenFiles asks for markdown files, starting in dir.
	OpenFiles(win Window, dir string) ([]string, error)
	// SaveFile asks where to save a file, and returns "" if none was
	// picked.
	SaveFile(win Window, s SaveFile) (string, error)
	// ChooseFolder asks for a folder, with title, starting in dir, over
	// the window the call ctx is for came from, and returns "" if none
	// was picked.
	ChooseFolder(ctx context.Context, title, dir string) (string, error)
	// Reveal shows file in the file manager.
	Reveal(file string) error
	// OpenURL opens a web address, as a link in a document gives it.
	OpenURL(url string) error
	// OpenFile opens a file that isn't markdown, as the system would.
	OpenFile(file string) error

	// ShowView says what the tab in front shows, for a View menu.
	ShowView(v View)
	// RecentChanged says the recent files changed, for an Open Recent
	// menu.
	RecentChanged()
	// Quit quits, after ShouldQuit has said yes.
	Quit()
}

// Window is one of a Host's windows.
type Window interface {
	ID() uint
	// Emit sends an event to the window's page.
	Emit(name string, data any)
	SetTitle(title string)
	Close()
	Focus()
	// MoveTo puts the window's top left corner at x, y on the screen.
	MoveTo(x, y int)
}

// Question is a question Host.Ask asks.
type Question struct {
	Title   string
	Message string
	Buttons []Button
}

// Button is one of a Question's answers.
type Button struct {
	Label   string
	Default bool // picked by return
	Cancel  bool // picked by escape
	Do      func()
}

// SaveFile is what Host.SaveFile asks.
type SaveFile struct {
	Message string
	Button  string // the button that saves, if not the usual one
	Dir     string
	Name    string
	Filter  string // the name of the files Pattern matches
	Pattern string // such as "*.md;*.markdown"
}
