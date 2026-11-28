package termstatus

import (
	"sort"
	"strings"
	"sync"
)

// StoreCap is how many records a Store keeps; past it, the one updated
// longest ago goes. The spec asks for at least MaxRecords.
const StoreCap = 256

// Store is the records of one terminal: what the programs on it have
// reported, one record per id, kept as the spec says. Its methods may be
// called from any goroutine.
type Store struct {
	mu   sync.Mutex
	recs map[string]*entry
	seq  uint64
}

type entry struct {
	Report
	seq uint64
}

// Apply takes a report in, replacing the record its id names, or for
// Clear removing that record and those under it (every record, with no
// id), and reports whether anything changed.
func (s *Store) Apply(r Report) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.State == Clear {
		return s.clear(r.ID)
	}
	if r.State == "" {
		return false
	}
	if s.recs == nil {
		s.recs = map[string]*entry{}
	}
	s.seq++
	if old, ok := s.recs[r.ID]; ok {
		changed := old.Report != r
		old.Report, old.seq = r, s.seq
		return changed
	}
	s.recs[r.ID] = &entry{Report: r, seq: s.seq}
	if len(s.recs) > StoreCap {
		var oldest string
		var at uint64
		for id, e := range s.recs {
			if at == 0 || e.seq < at {
				oldest, at = id, e.seq
			}
		}
		delete(s.recs, oldest)
	}
	return true
}

// clear removes id and the records under it; s.mu is held.
func (s *Store) clear(id string) bool {
	changed := false
	for k := range s.recs {
		if id == "" || k == id || strings.HasPrefix(k, id+"/") {
			delete(s.recs, k)
			changed = true
		}
	}
	return changed
}

// ProcessEnded drops what doesn't outlive the program, as a terminal
// does when the process exits or the shell prints its next prompt:
// Working and Blocked records, and Idle ones. Done and Error stay until
// something replaces or clears them. It reports whether anything
// changed.
func (s *Store) ProcessEnded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for k, e := range s.recs {
		switch e.State {
		case Working, Blocked, Idle:
			delete(s.recs, k)
			changed = true
		}
	}
	return changed
}

// Reset drops every record, as RIS does, and reports whether there were
// any.
func (s *Store) Reset() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	had := len(s.recs) > 0
	s.recs = nil
	return had
}

// Records is every record, the root first and then by id, each with the
// App of its nearest ancestor that has one when it has none itself.
func (s *Store) Records() []Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Report, 0, len(s.recs))
	for _, e := range s.recs {
		r := e.Report
		for p := r.ID; r.App == "" && p != ""; {
			i := strings.LastIndexByte(p, '/')
			if i < 0 {
				p = ""
			} else {
				p = p[:i]
			}
			if a, ok := s.recs[p]; ok {
				r.App = a.App
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Len is how many records there are.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.recs)
}
