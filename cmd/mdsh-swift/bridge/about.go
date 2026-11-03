package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/shellrc"
)

// about is what the Settings window's About tab shows about the bridge,
// as mdsh-gui's About. The app adds its own version.
type about struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildTime    string `json:"buildTime"`
	Modified     bool   `json:"modified"`
	GoVersion    string `json:"goVersion"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Home         string `json:"home"`
	SettingsFile string `json:"settingsFile"`
	Zshrc        string `json:"zshrc"`
	Tools        []tool `json:"tools"`
}

// tool is a program the app uses, and whether it was found on PATH.
type tool struct {
	Name    string `json:"name"`
	Note    string `json:"note"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

func (b *bridge) about() about {
	home, _ := os.UserHomeDir()
	zsh, _ := shellrc.Zsh.Path()
	ab := about{Version: "(devel)", GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, Home: home, SettingsFile: b.settings.Path(), Zshrc: zsh}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" {
			ab.Version = v
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				ab.Commit = s.Value
			case "vcs.time":
				ab.BuildTime = s.Value
			case "vcs.modified":
				ab.Modified = s.Value == "true"
			}
		}
	}
	tools := []tool{
		{Name: "zsh", Note: "runs zsh blocks, and sh blocks in a file that has zsh blocks"},
		{Name: "sh", Note: "runs sh blocks"},
		{Name: "mdsh", Note: "the command line tool, which the zsh wrapper and Create Report's command use"},
	}
	if shell := b.settings.Get().Shell; shell != "" {
		tools = append([]tool{{Name: shell, Note: "the shell for every file, from Settings"}}, tools...)
	}
	for _, t := range tools {
		if path, err := exec.LookPath(t.Name); err == nil {
			t.Path, t.Version = path, toolVersion(path)
		}
		ab.Tools = append(ab.Tools, t)
	}
	return ab
}

// toolVersion is the first line program prints for --version, or "".
func toolVersion(program string) string {
	if strings.HasSuffix(program, "/sh") {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, program, "--version").Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line
}

// zshWrapper is whether the user's .zshrc, and .bashrc if they have one,
// has the mdsh function that gives blocks the calling shell's aliases and
// functions, and loads mdsh's completion.
type zshWrapper struct {
	Path     string `json:"path"`
	BashPath string `json:"bashPath"`
	Home     string `json:"home"`
	On       bool   `json:"on"`
}

// zshWrapperState reads the .zshrc and .bashrc for the mdsh function,
// bringing a block from an older mdsh up to date as mdsh-gui does.
func zshWrapperState() (zshWrapper, error) {
	zsh, err := shellrc.Zsh.Path()
	if err != nil {
		return zshWrapper{}, err
	}
	home, _ := os.UserHomeDir()
	w := zshWrapper{Path: zsh, Home: home}
	if err := shellrc.Zsh.Update(zsh); err != nil {
		return w, err
	}
	if w.On, err = shellrc.Zsh.Installed(zsh); err != nil {
		return w, err
	}
	bash, err := shellrc.Bash.Path()
	if err != nil {
		return w, err
	}
	if _, err := os.Stat(bash); err != nil {
		return w, nil
	}
	w.BashPath = bash
	if err := shellrc.Bash.Update(bash); err != nil {
		return w, err
	}
	on, err := shellrc.Bash.Installed(bash)
	w.On = w.On || on
	return w, err
}

// setZshWrapper adds the mdsh function to the .zshrc, and to the .bashrc
// when there is one, or takes it out of both.
func setZshWrapper(on bool) (zshWrapper, error) {
	zsh, err := shellrc.Zsh.Path()
	if err != nil {
		return zshWrapper{}, err
	}
	if err := shellrc.Zsh.Set(zsh, on); err != nil {
		return zshWrapper{}, err
	}
	bash, err := shellrc.Bash.Path()
	if err != nil {
		return zshWrapper{}, err
	}
	if _, err := os.Stat(bash); err == nil {
		if err := shellrc.Bash.Set(bash, on); err != nil {
			return zshWrapper{}, err
		}
	}
	return zshWrapperState()
}
