package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The app menu's About opens the About tab of Settings, not the stock
// Cocoa About panel the About role shows.
func TestAppMenuAbout(t *testing.T) {
	// The roles below About need an application to name.
	application.New(application.Options{Name: "mdsh"})
	g := &gui{}
	m := application.NewMenu()
	g.addAppMenu(m)
	app := m.ItemAt(0).GetSubmenu()
	if app.FindByRole(application.About) != nil {
		t.Error("the app menu has the stock About item")
	}
	if app.FindByLabel("About mdsh") == nil {
		t.Error("the app menu has no About mdsh item")
	}
}
