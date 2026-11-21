package ui

import (
	"bytes"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestStartupTrace(t *testing.T) {
	launched := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	tr := startupTrace(strconv.FormatInt(launched.UnixNano(), 10), launched.Add(time.Second))
	if !tr.on || !tr.t0.Equal(launched) {
		t.Fatalf("t0 = %v, on %v; want the launch time", tr.t0, tr.on)
	}
	var out bytes.Buffer
	tr.w = &out
	tr.mark("main", launched.Add(12500*time.Microsecond))
	if got, want := out.String(), "mdsh startup     12.5ms main\n"; got != want {
		t.Errorf("mark printed %q, want %q", got, want)
	}
	if tr := startupTrace("1", launched); !tr.on || !tr.t0.Equal(launched) {
		t.Errorf("with 1, t0 = %v, on %v; want now", tr.t0, tr.on)
	}
	off := startupTrace("", launched)
	off.w = &out
	out.Reset()
	off.mark("main", launched)
	if off.on || out.Len() != 0 {
		t.Errorf("without the variable it printed %q", out.String())
	}
}

func TestServeStartupMarks(t *testing.T) {
	saved := startup
	defer func() { startup = saved }()
	launched := time.UnixMilli(1_800_000_000_000)
	var out bytes.Buffer
	startup = &startupTracer{on: true, t0: launched, w: &out}
	rec := httptest.NewRecorder()
	serveStartupMarks(rec, httptest.NewRequest("GET", startupPath+"?page=tab+1&m=script:1800000000250.5&m=bad&m=drawn:1800000000300", nil))
	if rec.Code != 204 {
		t.Errorf("status %d, want 204", rec.Code)
	}
	want := "mdsh startup    250.5ms tab 1 script\nmdsh startup    300.0ms tab 1 drawn\n"
	if out.String() != want {
		t.Errorf("printed %q, want %q", out.String(), want)
	}
}
