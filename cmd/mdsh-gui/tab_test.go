package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gdey/markdownshell/run"
)

func tabsOf(w *docWindow) []int {
	var ids []int
	for _, t := range w.tabs {
		ids = append(ids, t.id)
	}
	return ids
}

func TestWindowTabs(t *testing.T) {
	a, b, c := &docTab{id: 1}, &docTab{id: 2}, &docTab{id: 3}
	w := &docWindow{}
	w.insert(a, 0)
	w.insert(b, 1)
	w.insert(c, 9) // past the end goes last
	if got := tabsOf(w); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 || w.active != c {
		t.Fatalf("after inserts: %v, active %d", got, w.active.id)
	}
	if w.next(1) != a || w.next(-1) != b {
		t.Errorf("next from 3: %d, %d; want 1, 2", w.next(1).id, w.next(-1).id)
	}
	w.active = b
	if j := w.remove(b); j != 1 || w.active != c {
		t.Errorf("remove the tab in front: at %d, active %d; want 1, 3", j, w.active.id)
	}
	if j := w.remove(c); j != 1 || w.active != a {
		t.Errorf("remove the last tab, in front: at %d, active %d; want 1, 1", j, w.active.id)
	}
	if w.next(1) != nil {
		t.Error("next with one tab isn't nil")
	}
	if j := w.remove(c); j != -1 {
		t.Errorf("removing a tab not there: %d", j)
	}
	w.remove(a)
	if w.active != nil || len(w.tabs) != 0 {
		t.Errorf("emptied: %v, active %v", tabsOf(w), w.active)
	}
}

func TestTabCalls(t *testing.T) {
	var got any
	h := tabCalls(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Context().Value(tabKey{}) }))
	r := httptest.NewRequest("POST", "/wails/runtime", nil)
	r.Header.Set(tabHeader, "7")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != 7 {
		t.Errorf("with the header: %v, want 7", got)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/wails/runtime", nil))
	if got != nil {
		t.Errorf("without it: %v, want nil", got)
	}
}

// A tab keeps what a run sends until it is over, less questions already
// answered, for a page that starts partway through.
func TestTabLive(t *testing.T) {
	a := &App{tabs: map[int]*docTab{}, tabWindow: map[*docTab]*docWindow{}}
	tab := &docTab{app: a, id: 1, answers: make(chan run.Answer), done: make(chan struct{})}
	a.tabs[1] = tab
	ctx := context.WithValue(context.Background(), tabKey{}, 1)
	names := func() []string {
		live, err := a.Live(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var n []string
		for _, e := range live {
			n = append(n, e.Name)
		}
		return n
	}

	tab.emit("block", "before") // no run going: not kept
	err := tab.run(func() error {
		tab.emit("block", "started")
		answered := make(chan run.Answer)
		go func() { answered <- tab.confirm(0, "Run this block?") }()
		tab.answers <- run.Go // confirm has asked once this is taken
		if ans := <-answered; ans != run.Go {
			t.Errorf("answer %v, want Go", ans)
		}
		if got := names(); len(got) != 1 || got[0] != "block" {
			t.Errorf("after the answer: %v, want [block]", got)
		}
		tab.emit("menu", "save") // menu actions aren't part of the run
		tab.emit("block", "done")
		if got := names(); len(got) != 2 {
			t.Errorf("before the end: %v, want [block block]", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(); len(got) != 0 {
		t.Errorf("after the run: %v, want none", got)
	}
}
