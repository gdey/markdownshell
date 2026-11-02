module github.com/gdey/markdownshell/cmd/mdsh-gui

go 1.27

require (
	github.com/evanw/esbuild v0.28.2
	github.com/gdey/markdownshell v0.0.0
	github.com/neovim/go-client v1.2.1
	github.com/wailsapp/wails/v3 v3.0.0
	github.com/yuin/goldmark v1.7.17
	gopkg.in/yaml.v3 v3.0.1
	oss.terrastruct.com/d2 v0.7.2
)

require (
	github.com/PuerkitoBio/goquery v1.10.0 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/alecthomas/chroma/v2 v2.23.1 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/andybalholm/cascadia v1.3.2 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/dop251/goja v0.0.0-20240927123429-241b342198c2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-pdf/fpdf v0.9.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/pprof v0.0.0-20240927180334-d43a67379298 // indirect
	github.com/hinshun/vt10x v0.0.0-20220301184237-5011da428d02 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mazznoer/csscolorparser v0.1.5 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	oss.terrastruct.com/util-go v0.0.0-20250213174338-243d8661088a // indirect
)

// The GUI is built from the same tree as the library it shows.
replace github.com/gdey/markdownshell => ../..
