//go:build !windows

package ctl

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestServer(t *testing.T) {
	var got []string
	s, err := Listen(func(caller any, req Request) Response {
		var a NameArgs
		json.Unmarshal(req.Args, &a)
		got = append(got, caller.(string)+":"+req.Cmd+":"+a.Name)
		return Done("v-" + a.Name)
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.Path())
	for path, want := range map[string]os.FileMode{dir: 0o700, s.Path(): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != want {
			t.Errorf("%s: mode %o, want %o", path, perm, want)
		}
	}

	tok := s.Issue("blk")
	resp, err := Call(s.Path(), tok, "get", NameArgs{Name: "x"})
	if err != nil || !resp.OK || resp.Value != "v-x" || resp.V != Version {
		t.Errorf("get: %+v, %v", resp, err)
	}
	if resp, err := Call(s.Path(), "nope", "get", NameArgs{Name: "x"}); err != nil || resp.OK || resp.Code != Refused {
		t.Errorf("unknown token: %+v, %v", resp, err)
	}
	s.Revoke(tok)
	if resp, err := Call(s.Path(), tok, "get", NameArgs{Name: "y"}); err != nil || resp.OK || resp.Code != Refused {
		t.Errorf("revoked token: %+v, %v", resp, err)
	}

	// A request in another version is refused.
	tok = s.Issue("blk")
	conn, err := net.Dial("unix", s.Path())
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte(`{"v":2,"token":"` + tok + `","cmd":"get"}` + "\n"))
	var r Response
	if err := json.NewDecoder(conn).Decode(&r); err != nil || r.OK || r.Code != Refused {
		t.Errorf("v2: %+v, %v", r, err)
	}
	conn.Close()

	if len(got) != 1 || got[0] != "blk:get:x" {
		t.Errorf("handler saw %q", got)
	}

	if err := s.Close(); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("socket dir left behind: %v", err)
	}
	if _, err := Call(s.Path(), tok, "get", NameArgs{}); err == nil {
		t.Error("call after Close worked")
	}
}

func TestCallNotRunning(t *testing.T) {
	if _, err := Call("", "t", "get", nil); err != ErrNotRunning {
		t.Errorf("empty path: %v", err)
	}
	if _, err := Call(filepath.Join(t.TempDir(), "none.sock"), "t", "get", nil); !errors.Is(err, ErrNotRunning) {
		t.Errorf("no listener: %v", err)
	}
}
