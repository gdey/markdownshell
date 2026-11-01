package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLog(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	l.now = func() time.Time { return time.Date(2026, 11, 1, 10, 0, 0, 5e6, time.UTC) }
	l.Event("block.start", Fields{"block": "a", "env": ""})
	l.Event("var", Fields{"name": "MDSH_VAR_x", "new": l.Secret("hunter2"), "old": l.Secret("hunter2")})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if want := `{"v":1,"seq":1,"t":"2026-11-01T10:00:00.005Z","event":"block.start","block":"a","env":""}`; lines[0] != want {
		t.Errorf("line 1 = %s\nwant %s", lines[0], want)
	}
	if strings.Contains(buf.String(), "hunter2") {
		t.Error("secret in the log")
	}
	var e struct {
		Seq      int
		New, Old map[string]any
	}
	if err := json.Unmarshal([]byte(lines[1]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Seq != 2 || e.New["secret"] != true || e.New["hmac"] == "" || e.New["hmac"] != e.Old["hmac"] {
		t.Errorf("event 2 = %+v", e)
	}
	if New(&buf).Secret("hunter2").(map[string]any)["hmac"] == e.New["hmac"] {
		t.Error("two logs share a key")
	}
	var nilLog *Log
	nilLog.Event("x", nil) // doesn't panic
}
