package env

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Proc is a process that a shell started, directly or through others,
// and the processes it started in turn.
type Proc struct {
	PID int `json:"pid"`
	// Command is the command line, as ps shows it.
	Command string `json:"command"`
	// Secs is how long the process has run, in seconds.
	Secs int `json:"secs"`
	// Block names the background block the process is, if it is one.
	Block    string  `json:"block,omitempty"`
	Children []*Proc `json:"children,omitempty"`
}

// ShellProcs is an env's shell and the processes under it.
type ShellProcs struct {
	// Env is the env's name, "" for the top level.
	Env      string  `json:"env"`
	PID      int     `json:"pid"`
	Secs     int     `json:"secs"`
	Children []*Proc `json:"children,omitempty"`
}

// psRow is one line of ps.
type psRow struct {
	pid, ppid int
	running   time.Duration
	command   string
}

// parsePS reads the lines of ps -A -o pid=,ppid=,stat=,etime=,args=.
// A zombie, which has ended but whose parent hasn't waited for it yet,
// is left out: there is nothing left of it to kill.
func parsePS(out string) map[int]psRow {
	rows := map[int]psRow{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || strings.HasPrefix(f[2], "Z") {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		d, ok := parseEtime(f[3])
		if err1 != nil || err2 != nil || !ok {
			continue
		}
		rows[pid] = psRow{pid, ppid, d, strings.Join(f[4:], " ")}
	}
	return rows
}

// parseEtime reads ps's etime, [[dd-]hh:]mm:ss.
func parseEtime(s string) (time.Duration, bool) {
	days := 0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, false
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	secs := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, false
		}
		secs = secs*60 + n
	}
	return time.Duration(days*86400+secs) * time.Second, true
}

// tree builds the processes under pid from rows, oldest first.
func tree(rows map[int]psRow, kids map[int][]int, pid int) []*Proc {
	var out []*Proc
	for _, c := range kids[pid] {
		r := rows[c]
		out = append(out, &Proc{PID: c, Command: r.command, Secs: int(r.running / time.Second), Children: tree(rows, kids, c)})
	}
	slices.SortFunc(out, func(a, b *Proc) int {
		if a.Secs != b.Secs {
			return b.Secs - a.Secs
		}
		return a.PID - b.PID
	})
	return out
}

func children(rows map[int]psRow) map[int][]int {
	kids := map[int][]int{}
	for _, r := range rows {
		kids[r.ppid] = append(kids[r.ppid], r.pid)
	}
	return kids
}

// Processes lists the stack's shells, those still starting too, each
// with the processes under it:
// what blocks are running, background blocks, and anything a block or
// the History pane left running, such as cmd &. It is empty where ps
// can't be read.
func (s *Stack) Processes() []ShellProcs {
	shells := s.shells()
	names := slices.Sorted(maps.Keys(shells))
	rows := psRows()
	kids := children(rows)
	var out []ShellProcs
	for _, name := range names {
		sh := shells[name]
		pid := sh.PID()
		if pid == 0 || sh.Exited() {
			continue
		}
		procs := tree(rows, kids, pid)
		// A job on a terminal of its own is mdsh's child, not the
		// shell's, but it is the shell's background block.
		for _, j := range sh.termJobPIDs() {
			if r, ok := rows[j]; ok {
				procs = append(procs, &Proc{PID: j, Command: r.command, Secs: int(r.running / time.Second), Children: tree(rows, kids, j)})
			}
		}
		out = append(out, ShellProcs{Env: name, PID: pid, Secs: int(rows[pid].running / time.Second), Children: procs})
	}
	return out
}

// KillProcess kills process pid and what it started, if it runs under
// one of the stack's shells, and reports whether it did. The shells
// themselves are not killed this way: Restart Shells ends those.
func (s *Stack) KillProcess(pid int) bool {
	for _, sh := range s.Processes() {
		if under(sh.Children, pid) {
			killTree(pid)
			return true
		}
	}
	return false
}

func under(ps []*Proc, pid int) bool {
	for _, p := range ps {
		if p.PID == pid || under(p.Children, pid) {
			return true
		}
	}
	return false
}
