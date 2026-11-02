package main

import (
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

func TestFit(t *testing.T) {
	area := application.Rect{Width: 1440, Height: 875}
	for _, c := range []struct {
		w, h   int
		area   application.Rect
		share  float64
		ww, wh int
	}{
		{1280, 900, area, fitShare, 1280, 787},
		{800, 600, area, fitShare, 800, 600},
		{2000, 2000, area, 1, 1440, 875},
		{2000, 2000, application.Rect{}, fitShare, 2000, 2000},
	} {
		if w, h := fit(c.w, c.h, c.area, c.share); w != c.ww || h != c.wh {
			t.Errorf("fit(%d, %d, %v, %v) = %d, %d, want %d, %d", c.w, c.h, c.area, c.share, w, h, c.ww, c.wh)
		}
	}
}

func TestOnScreen(t *testing.T) {
	left := application.Rect{X: 0, Y: 25, Width: 1440, Height: 875}
	right := application.Rect{X: 1440, Y: 0, Width: 1920, Height: 1080}
	areas := []application.Rect{left, right}
	for _, c := range []struct {
		name string
		p    settings.Place
		ok   bool
		area application.Rect
	}{
		{"on the left", settings.Place{X: 100, Y: 100, Width: 800, Height: 600}, true, left},
		{"on the right", settings.Place{X: 2000, Y: 100, Width: 800, Height: 600}, true, right},
		{"title bar just showing", settings.Place{X: 1340, Y: 100, Width: 800, Height: 600}, true, left},
		{"unplugged display", settings.Place{X: 4000, Y: 100, Width: 800, Height: 600}, false, application.Rect{}},
		{"above the screens", settings.Place{X: 100, Y: -700, Width: 800, Height: 600}, false, application.Rect{}},
		{"sliver at the edge", settings.Place{X: 3300, Y: 100, Width: 800, Height: 600}, false, application.Rect{}},
	} {
		area, ok := onScreen(c.p, areas)
		if ok != c.ok || area != c.area {
			t.Errorf("%s: onScreen = %v, %v, want %v, %v", c.name, area, ok, c.area, c.ok)
		}
	}
}

func TestPlaceFor(t *testing.T) {
	st, err := settings.Open(filepath.Join(t.TempDir(), "gui.json"))
	if err != nil {
		t.Fatal(err)
	}
	saved := settings.Place{File: "/d/a.md", X: 200, Y: 100, Width: 1000, Height: 700}
	if err := st.Update(func(s *settings.Settings) {
		s.Placed(saved)
		s.Placed(settings.Place{File: "/d/gone.md", X: 5000, Y: 100, Width: 1000, Height: 700})
		s.Placed(settings.Place{File: "/d/big.md", X: 0, Y: 25, Width: 3000, Height: 2000})
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{settings: st}
	screen := []application.Rect{{X: 0, Y: 25, Width: 1440, Height: 875}}
	def := settings.Place{Width: 1280, Height: 787}
	// Off the screens, a window that goes back where it was opens
	// centered at the default size; one that only takes its size back
	// keeps that.
	gone, goneMove := settings.Place{File: "/d/gone.md", Width: 1280, Height: 787}, false
	if !keepPlace {
		gone, goneMove = settings.Place{File: "/d/gone.md", X: 5000, Y: 100, Width: 1000, Height: 700}, true
	}
	for _, c := range []struct {
		name  string
		file  string
		areas []application.Rect
		want  settings.Place
		move  bool
	}{
		{"start screen", "", screen, def, false},
		{"never opened", "/d/new.md", screen, settings.Place{File: "/d/new.md", Width: 1280, Height: 787}, false},
		{"remembered", "/d/a.md", screen, saved, true},
		{"before the screens are known", "/d/a.md", nil, saved, true},
		{"no screens, nothing remembered", "", nil, settings.Place{Width: docWidth, Height: docHeight}, false},
		{"off the screens", "/d/gone.md", screen, gone, goneMove},
		{"bigger than the screen", "/d/big.md", screen, settings.Place{File: "/d/big.md", X: 0, Y: 25, Width: 1440, Height: 875}, true},
	} {
		p, move := a.placeFor(c.file, c.areas)
		if p != c.want || move != c.move {
			t.Errorf("%s: placeFor = %+v, %v, want %+v, %v", c.name, p, move, c.want, c.move)
		}
	}
}
