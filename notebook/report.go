package notebook

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/report"
	"github.com/gdey/markdownshell/run"
)

// Report is what the Report dialog asks for.
type Report struct {
	Format     report.Format `json:"format"`
	HideCode   bool          `json:"hideCode"`
	HideOutput bool          `json:"hideOutput"`
	// Saved reports the file as last saved, running nothing, like
	// mdsh -n. Otherwise the report shows this window's output. "Run
	// all first" is a run, then a report of the window.
	Saved bool `json:"saved"`
	// Ran says the window's output comes from running every block, for
	// the command Command shows.
	Ran bool `json:"ran"`
	// Dest is the file to write.
	Dest string `json:"dest"`
}

// Ran counts the runnable blocks that have run in this window, and all
// of them.
func (n *Notebook) Ran() (ran, total int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, b := range n.d.Blocks {
		if !run.Runnable(b) {
			continue
		}
		total++
		if r := n.results[i]; r != nil && r.Ran {
			ran++
		}
	}
	return ran, total
}

// MakeReport renders the report r asks for. Image links are read from
// the file's folder, where runs write them.
func (n *Notebook) MakeReport(r Report) ([]byte, error) {
	if r.Format == "" {
		r.Format = report.Markdown
	}
	opts := report.Options{
		Format:     r.Format,
		HideCode:   r.HideCode,
		HideOutput: r.HideOutput,
		// The report's conditions see $MDSH_MODE report, so
		// show={{$MDSH_MODE}}!=report leaves a block out of it.
		Env:   run.WithMode(os.Environ(), run.ModeReport),
		Title: report.Title(n.File()),
		Dir:   filepath.Dir(n.File()),
	}
	if r.Saved {
		src, err := os.ReadFile(n.File())
		if err != nil {
			return nil, err
		}
		d, err := doc.Parse(src)
		if err != nil {
			return nil, err
		}
		return report.Render(d, run.Includes(d, run.Options{File: n.File()}), opts)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return nil, ErrBusy
	}
	return report.Render(n.d, n.results, opts)
}

// WriteReport writes the report r asks for to r.Dest.
func (n *Notebook) WriteReport(r Report) error {
	if r.Dest == "" {
		return fmt.Errorf("no file to write the report to")
	}
	out, err := n.MakeReport(r)
	if err != nil {
		return err
	}
	return os.WriteFile(r.Dest, out, 0o644)
}

// ReportCommand is the mdsh command that makes the same report, run in
// the document's folder: files there are named on their own, and other
// paths under home as ~/…. For a report of the window's output, it is
// the command for the saved file, which is the same once it is saved.
func (n *Notebook) ReportCommand(r Report, home string) string {
	ro := n.RunOptions()
	return report.Command{
		File:        n.File(),
		Dest:        r.Dest,
		Format:      r.Format,
		HideCode:    r.HideCode,
		HideOutput:  r.HideOutput,
		Run:         r.Ran && !r.Saved,
		Args:        ro.Args,
		Only:        ro.Only,
		Step:        ro.Step,
		StopOnError: ro.StopOnError,
		Home:        home,
	}.String()
}
