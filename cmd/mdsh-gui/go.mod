module github.com/gdey/markdownshell/cmd/mdsh-gui

go 1.27

require (
	github.com/evanw/esbuild v0.28.2
	github.com/gdey/markdownshell v0.0.0
	github.com/wailsapp/wails/v3 v3.0.0
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-pdf/fpdf v0.9.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/yuin/goldmark v1.7.17 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// The GUI is built from the same tree as the library it shows.
replace github.com/gdey/markdownshell => ../..
