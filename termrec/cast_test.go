package termrec

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCastRoundTrip(t *testing.T) {
	rec := Recording{Cols: 20, Rows: 4, Start: time.Unix(1700000000, 0), Events: []Event{
		{At: 0, Data: []byte("hi \x1b[31mred\x1b[0m\r\n")},
		{At: 1500 * time.Millisecond, Data: []byte("caf\xc3")}, // é split across writes
		{At: 2 * time.Second, Data: []byte("\xa9\r\n")},
	}}
	var b bytes.Buffer
	if err := WriteCast(&b, rec); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if lines[0] != `{"version":2,"width":20,"height":4,"timestamp":1700000000}` {
		t.Errorf("header = %s", lines[0])
	}
	if want := `[1.5,"o","caf"]`; lines[2] != want {
		t.Errorf("line 3 = %s, want %s", lines[2], want)
	}
	got, err := ReadCast(&b)
	if err != nil {
		t.Fatal(err)
	}
	want := rec
	want.Events = []Event{rec.Events[0], {At: 1500 * time.Millisecond, Data: []byte("caf")}, {At: 2 * time.Second, Data: []byte("é\r\n")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
}

func TestReadCastSkipsInput(t *testing.T) {
	cast := `{"version":2,"width":10,"height":2}
[0.1,"i","x"]
[0.2,"o","y"]
`
	rec, err := ReadCast(strings.NewReader(cast))
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Events) != 1 || string(rec.Events[0].Data) != "y" {
		t.Errorf("events = %+v", rec.Events)
	}
}

func TestIsCast(t *testing.T) {
	var b bytes.Buffer
	WriteCast(&b, rec("hi"))
	for _, tc := range []struct {
		data string
		want bool
	}{
		{b.String(), true},
		{`{"version":1,"width":80}` + "\n", false},
		{`{"version":2}` + "\n", false},
		{"GIF89a", false},
		{"", false},
		{`{"version":2,"width":80,"height":24}`, true},
	} {
		if got := IsCast([]byte(tc.data)); got != tc.want {
			t.Errorf("IsCast(%.30q) = %v", tc.data, got)
		}
	}
}

// A cast made to be huge is refused before anything is allocated for
// it (security review v1.4.0, finding 4).
func TestReadCastLimits(t *testing.T) {
	for _, src := range []string{
		`{"version":2,"width":100000,"height":100000}` + "\n",
		`{"version":2,"width":80,"height":501}` + "\n",
		`{"version":2,"width":-1,"height":24}` + "\n",
	} {
		if _, err := ReadCast(strings.NewReader(src)); err == nil {
			t.Errorf("ReadCast(%q) = nil error", src)
		}
	}
	var b strings.Builder
	b.WriteString(`{"version":2,"width":80,"height":24}` + "\n")
	for range MaxEvents + 1 {
		b.WriteString(`[0,"o","x"]` + "\n")
	}
	if _, err := ReadCast(strings.NewReader(b.String())); err == nil || !strings.Contains(err.Error(), "events") {
		t.Errorf("too many events: err = %v", err)
	}
}

// Past maxCells, shots keeps the last screen and drops the ones before.
func TestShotsBudget(t *testing.T) {
	rec := Recording{Cols: MaxCols, Rows: MaxRows}
	for i := range 200 {
		rec.Events = append(rec.Events, Event{At: time.Duration(i) * time.Second, Data: []byte(strings.Repeat("x", i+1) + "\r\n")})
	}
	frames, _, _ := shots(rec)
	if n := len(frames); n*MaxCols*MaxRows > maxCells || n < 2 {
		t.Fatalf("%d frames of %d cells, budget %d", n, MaxCols*MaxRows, maxCells)
	}
	if last := frames[len(frames)-1]; last.at != 199*time.Second {
		t.Errorf("last frame at %v, want the last write", last.at)
	}
}
