package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Startup tracing: with MDSH_STARTUP_TRACE set, mdsh-gui prints to stderr
// when it gets to each point of starting up, Go's and its pages', as
//
//	mdsh startup    123.4ms window shown
//
// The times are from when the process was launched if the variable holds
// that time in Unix nanoseconds, as cmd/mdsh-gui/internal/startuptime
// sets it, and otherwise from when this package's variables were set,
// which is after Go has initialized the packages it imports. Without the
// variable, StartupMark does nothing.
var startup = startupTrace(os.Getenv("MDSH_STARTUP_TRACE"), time.Now())

type startupTracer struct {
	on bool
	t0 time.Time
	mu sync.Mutex
	w  io.Writer
}

func startupTrace(v string, now time.Time) *startupTracer {
	t := &startupTracer{on: v != "", t0: now, w: os.Stderr}
	// Anything before 2001 isn't a launch time in nanoseconds.
	if ns, err := strconv.ParseInt(v, 10, 64); err == nil && ns > 1e18 {
		t.t0 = time.Unix(0, ns)
	}
	return t
}

// StartupMark notes, with MDSH_STARTUP_TRACE set, that starting up got
// to what now.
func StartupMark(what string) { startup.mark(what, time.Now()) }

// StartupTracing reports whether MDSH_STARTUP_TRACE is set.
func StartupTracing() bool { return startup.on }

func (t *startupTracer) mark(what string, at time.Time) {
	if !t.on {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.w, "mdsh startup %8.1fms %s\n", float64(at.Sub(t.t0).Microseconds())/1000, what)
}

// startupPath is where a page sends its own marks once it has drawn
// (see web/src/startup.ts): page names the page, and each m is a
// mark's name, a colon and its time in Unix milliseconds.
const startupPath = "/startup-marks"

// serveStartupMarks prints the marks a page sent, when tracing.
func serveStartupMarks(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if !startup.on {
		return
	}
	q := r.URL.Query()
	page := q.Get("page")
	for _, m := range q["m"] {
		name, at, ok := strings.Cut(m, ":")
		ms, err := strconv.ParseFloat(at, 64)
		if !ok || err != nil {
			continue
		}
		startup.mark(page+" "+name, time.UnixMicro(int64(ms*1000)))
	}
}
