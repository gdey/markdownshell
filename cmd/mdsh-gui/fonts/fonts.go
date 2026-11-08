// Package fonts lists the font families installed on this computer, for
// the Settings tab's font menus.
package fonts

import (
	"slices"
	"strings"
	"unicode"
)

// Fonts are the installed font families.
type Fonts struct {
	// All is every family; Mono those whose letters are all one width,
	// for the code font. Both are sorted, and empty where the system
	// can't be asked, so Settings falls back to its list of common ones.
	All  []string `json:"all"`
	Mono []string `json:"mono"`
}

// List asks the system for its font families.
func List() Fonts { return systemFonts() }

// Name is a font family name as the page can take it in a CSS
// variable: letters, digits, spaces, dots, hyphens and underscores,
// trimmed; anything else gives "", the look's own font.
func Name(name string) string {
	name = strings.TrimSpace(name)
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" .-_", r) {
			return ""
		}
	}
	return name
}

// tidyFonts makes a family list fit for a menu: names Settings can save
// (see Name), none hidden (a leading "."), sorted and without
// repeats.
func tidyFonts(names []string) []string {
	var out []string
	for _, n := range names {
		n = Name(n)
		if n == "" || strings.HasPrefix(n, ".") {
			continue
		}
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return slices.Compact(out)
}

// withNerdFonts adds to mono the Nerd Font families in all: their
// letters are one width, but a family without "Mono" has wide icons,
// so the system may not call it fixed pitch.
func withNerdFonts(all, mono []string) []string {
	for _, n := range all {
		if strings.Contains(n, "Nerd Font") && !strings.HasPrefix(n, "Symbols Nerd Font") {
			mono = append(mono, n)
		}
	}
	mono = tidyFonts(mono)
	return slices.DeleteFunc(mono, func(n string) bool { return !slices.Contains(all, n) })
}
