// Command startuptime launches mdsh-gui several times and reports how
// long it took to get to each point of starting up, from the marks it
// prints with MDSH_STARTUP_TRACE set (see ui/startup.go): the median
// and the fastest run of each.
//
//	go build -o /tmp/mdsh-gui . && go run ./internal/startuptime [-n 10] /tmp/mdsh-gui [file.md | folder]
//
// Each run starts with a home folder of its own, so no settings, recent
// files or window places carry over, and ends, killed, once the first
// tab has drawn. Close nothing by hand while it runs.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var markLine = regexp.MustCompile(`^mdsh startup +([0-9.]+)ms (.*)$`)

func main() {
	n := flag.Int("n", 10, "how many times to start it")
	wait := flag.Duration("timeout", 30*time.Second, "how long to wait for a run's first tab to draw")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: startuptime [-n 10] path/to/mdsh-gui [args...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	times := map[string][]float64{}
	var order []string
	for i := 0; i < *n; i++ {
		marks, err := once(flag.Args(), *wait)
		if err != nil {
			fmt.Fprintf(os.Stderr, "run %d: %v\n", i+1, err)
			os.Exit(1)
		}
		for _, m := range marks {
			if _, ok := times[m.name]; !ok {
				order = append(order, m.name)
			}
			times[m.name] = append(times[m.name], m.ms)
		}
	}
	slices.SortStableFunc(order, func(a, b string) int {
		return int(median(times[a])*10 - median(times[b])*10)
	})
	fmt.Printf("%d runs of %s\n%10s %10s  %s\n", *n, strings.Join(flag.Args(), " "), "median", "fastest", "mark")
	for _, name := range order {
		fmt.Printf("%8.1fms %8.1fms  %s\n", median(times[name]), slices.Min(times[name]), name)
	}
}

type mark struct {
	name string
	ms   float64
}

// once starts the program once and returns its marks, up to the first
// tab's page having drawn.
func once(args []string, wait time.Duration) ([]mark, error) {
	home, err := os.MkdirTemp("", "startuptime-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME=", "MDSH_STARTUP_TRACE="+strconv.FormatInt(time.Now().UnixNano(), 10))
	errs, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	got := make(chan []mark, 1)
	go func() {
		var marks []mark
		s := bufio.NewScanner(errs)
		for s.Scan() {
			m := markLine.FindStringSubmatch(s.Text())
			if m == nil {
				continue
			}
			ms, _ := strconv.ParseFloat(m[1], 64)
			marks = append(marks, mark{m[2], ms})
			if strings.HasPrefix(m[2], "tab ") && strings.HasSuffix(m[2], " drawn") {
				got <- marks
				return
			}
		}
		got <- nil
	}()
	select {
	case marks := <-got:
		if marks == nil {
			return nil, fmt.Errorf("it ended before its first tab drew")
		}
		return marks, nil
	case <-time.After(wait):
		return nil, fmt.Errorf("its first tab hadn't drawn after %v", wait)
	}
}

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}
