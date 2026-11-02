package ctl

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrNotRunning is Call's error when there is no run to call: the socket
// path is empty or nothing is listening on it.
var ErrNotRunning = errors.New("not running under mdsh")

// Call sends cmd with args to the socket at path, as the block that has
// token, and returns the answer. It waits as long as the run takes to
// answer, which for a question can be a while.
func Call(path, token, cmd string, args any) (Response, error) {
	if path == "" {
		return Response{}, ErrNotRunning
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return Response{}, err
	}
	req, err := json.Marshal(Request{V: Version, Token: token, Cmd: cmd, Args: raw})
	if err != nil {
		return Response{}, err
	}
	conn, err := dial(path)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(readTimeout))
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return Response{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return Response{}, fmt.Errorf("no answer from mdsh: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("bad answer from mdsh: %w", err)
	}
	if !resp.OK && resp.Code == 0 {
		resp.Code = Refused
	}
	return resp, nil
}
