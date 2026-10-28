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
