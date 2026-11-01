package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// folder is a folder's listing, for the path prompt in runtime.js.
type folder struct {
	Dir     string  `json:"dir"`
	Parent  string  `json:"parent,omitempty"` // "" at the root
	Entries []entry `json:"entries"`
}

type entry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

// files lists the folder dir names: its folders first, then the files
// pattern matches, or none for a folder prompt. Hidden ones are left
// out.
func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir, err := s.absPath(q.Get("dir"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	f := folder{Dir: dir, Entries: []entry{}}
	if p := filepath.Dir(dir); p != dir {
		f.Parent = p
	}
	var patterns []string
	for p := range strings.SplitSeq(q.Get("pattern"), ";") {
		if p = strings.TrimSpace(p); p != "" && p != "*" {
			patterns = append(patterns, p)
		}
	}
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		isDir := de.IsDir()
		if de.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = fi.IsDir()
			}
		}
		if !isDir && (q.Get("kind") == "folder" || !matches(patterns, name)) {
			continue
		}
		f.Entries = append(f.Entries, entry{Name: name, Dir: isDir})
	}
	slices.SortStableFunc(f.Entries, func(a, b entry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f)
}

// matches reports whether name matches one of patterns, or there are
// none.
func matches(patterns []string, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}
