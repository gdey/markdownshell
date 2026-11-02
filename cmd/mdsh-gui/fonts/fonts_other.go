//go:build !darwin

package fonts

import (
	"os/exec"
	"strings"
)

// systemFonts asks fontconfig's fc-list for the installed families.
// Without fc-list, as on Windows, it lists none.
func systemFonts() Fonts {
	all := tidyFonts(fcList(":"))
	if len(all) == 0 {
		return Fonts{}
	}
	return Fonts{All: all, Mono: withNerdFonts(all, fcList(":spacing=mono"))}
}

// fcList runs fc-list for the families matching pattern. A font with
// several names lists them with commas; the first is its own.
func fcList(pattern string) []string {
	out, err := exec.Command("fc-list", pattern, "family").Output()
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		name, _, _ := strings.Cut(line, ",")
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}
