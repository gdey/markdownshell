// Package ctl is the control socket that lets a block talk back to the
// run that started it: mdsh ctl on one side, the run on the other.
//
// Each call opens a connection, sends one line of JSON, reads one line
// of JSON back, and closes. The server handles calls one at a time, in
// the order they arrive.
package ctl

import (
	"encoding/json"
	"fmt"
)

// Version is the protocol version this package speaks.
const Version = 1

// The variables mdsh exports in every block's prelude.
const (
	// SockVar holds the socket's path.
	SockVar = "MDSH_SOCK"
	// TokenVar holds the token for this run of this block.
	TokenVar = "MDSH_TOKEN"
)

// Exit codes, the same for every command.
const (
	// OK is done, or confirm was answered yes.
	OK = 0
	// No is confirm answered no.
	No = 1
	// Refused is bad arguments, an unknown task, a token that has
	// stopped working, or a name that an input binds.
	Refused = 2
	// NotRunning is $MDSH_SOCK unset, or nothing listening on it.
	NotRunning = 3
	// NoAnswer is ask or confirm with nobody to answer and no default.
	NoAnswer = 4
)

// Request is one call.
type Request struct {
	V     int             `json:"v"`
	Token string          `json:"token"`
	Cmd   string          `json:"cmd"`
	Args  json.RawMessage `json:"args,omitempty"`
}

// Response is the answer to one call. Value is what the command prints,
// such as the value get read.
type Response struct {
	V     int    `json:"v"`
	OK    bool   `json:"ok"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
	Code  int    `json:"code,omitempty"`
}

// Done is a response for a call that succeeded and prints value.
func Done(value string) Response { return Response{V: Version, OK: true, Value: value} }

// Refuse is a response for a call that was refused, with code Refused.
func Refuse(format string, a ...any) Response {
	return Response{V: Version, Error: fmt.Sprintf(format, a...), Code: Refused}
}

// SetArgs are the arguments to set.
type SetArgs struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

// NameArgs are the arguments to unset and get.
type NameArgs struct {
	Name string `json:"name"`
}

// StatusArgs are the arguments to status. Step and Of are 0 when the
// call gave no count.
type StatusArgs struct {
	Text string `json:"text"`
	Step int    `json:"step,omitempty"`
	Of   int    `json:"of,omitempty"`
}
